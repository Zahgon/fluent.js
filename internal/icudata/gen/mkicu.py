#!/usr/bin/env python3
"""Export the ICU data the Go library needs from the node binary into internal/icudata/icu.zip.

Usage, in the pinned Node image (node 24.22.1, ICU 78.3, CLDR 48.0, tz 2026e):
    finddat.py /usr/local/bin/node out/icudt78l.dat
    mkicu.py out/icudt78l.dat OUTPUT.zip /usr/local/bin/node

Every entry is compact JSON: main/<locale>, curr/<locale>, unit/<locale>, zone/<locale>, and supp/<name> for
data that is not per locale. Only what the Intl emulation reads is kept.
"""
from __future__ import annotations

import json
import re
import subprocess
import sys
import zipfile

from bytestrie import entries
from fallbacktable import default_scripts
from resbundle import Package

NON_LOCALE = {'pool', 'res_index', 'plurals', 'supplementalData', 'metadata', 'langInfo', 'numberingSystems', 'dayPeriods',
              'metaZones', 'zoneinfo64', 'timezoneTypes', 'windowsZones', 'keyTypeData', 'genderList', 'units', 'icustd',
              'icuver', 'currencyNumericCodes', 'likelySubtags', 'tzdbNames', 'grammaticalFeatures', 'pluralRanges'}
DROP_CALENDARS = {'chinese', 'dangi'}
CALENDAR_DROP_KEYS = {'intervalFormats', 'quarters', 'DateTimePatterns%relative', 'cyclicNameSets', 'monthPatterns'}


def dumps(value) -> bytes:
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'), sort_keys=False).encode('utf-8', 'surrogatepass')


def node(script: str, *args: str) -> str:
    return subprocess.run(['node', '-e', script, *args], check=True, capture_output=True, text=True).stdout


def locale_names(pkg: Package, prefix: str) -> list[str]:
    names = []
    for item in pkg.items:
        if not item.startswith(prefix) or not item.endswith('.res'):
            continue
        base = item[len(prefix):-4]
        if '/' in base or base in NON_LOCALE:
            continue
        names.append(base)
    return sorted(names)


def trim_main(res: dict) -> dict:
    out = {}
    for key in ('%%Parent', '%%ALIAS', 'NumberElements'):
        if key in res:
            out[key] = res[key]
    if 'calendar' in res:
        cals = {}
        for name, cal in res['calendar'].items():
            if name in DROP_CALENDARS:
                continue
            cals[name] = {k: v for k, v in cal.items() if k not in CALENDAR_DROP_KEYS} if isinstance(cal, dict) and '$alias' not in cal else cal
        out['calendar'] = cals
    if 'fields' in res:
        fields = {}
        for name, field in res['fields'].items():
            if isinstance(field, dict) and 'dn' in field:
                fields[name] = {'dn': field['dn']}
            elif isinstance(field, dict) and '$alias' in field:
                fields[name] = field
        if fields:
            out['fields'] = fields
    return out


def trim_units(node_value):
    if isinstance(node_value, dict):
        return {k: trim_units(v) for k, v in node_value.items() if k not in ('case', 'gender')}
    return node_value


def trie_tree(raw: bytes) -> dict:
    """The trie as {language: {"v": value, "n": {script: {"v":..., "n": {region: value}}}}}, keys in full ("*" for empty)."""
    tree: dict = {}
    for key, value, _final in entries(raw):
        parts: list[str] = []
        cur = ''
        for b in key:
            if b & 0x80:
                parts.append(cur + chr(b & 0x7f))
                cur = ''
            elif b == 0x2a:
                parts.append('*')
            else:
                cur += chr(b)
        node_ = tree
        for i, part in enumerate(parts):
            entry = node_.setdefault(part, {})
            if i == len(parts) - 1:
                entry['v'] = value
            else:
                node_ = entry.setdefault('n', {})
    return tree


def likely(pkg: Package) -> dict:
    info = pkg.bundle('langInfo.res').read()['likely']
    raw = bytes.fromhex(info['trie']['$binary'])
    tree = trie_tree(raw)
    m49 = info['m49']
    nums = info['lsrnum']
    # The script subtag is stored as an ICU script code; each code in use is named by asking Intl.Locale about a tag
    # the trie maps to that code.
    samples: dict[int, str] = {}

    def visit(table, path):
        for part, entry in table.items():
            sub = path + ([] if part == '*' else [part])
            v = entry.get('v', 0)
            if v > 1:
                tag = '-'.join(sub)
                samples.setdefault((nums[v] >> 24) & 0xff, tag if tag[:1].islower() else '-'.join(['und'] + sub))
            if 'n' in entry:
                visit(entry['n'], sub)

    visit(tree, [])
    script = 'console.log(JSON.stringify(JSON.parse(process.argv[1]).map(t => new Intl.Locale(t).maximize().script)))'
    codes = sorted(samples)
    named = json.loads(node(script, json.dumps([samples[c] for c in codes])))
    code_to_script = dict(zip(codes, named))

    def lsr(v: int) -> list[str]:
        if v in (0, 1):
            return ['', '', '']
        low = v & 0x00ffffff
        lang_code = low % (27 * 27 * 27)
        lang = chr(ord('a') + lang_code % 27 - 1) + chr(ord('a') + (lang_code // 27) % 27 - 1)
        if lang_code // (27 * 27):
            lang += chr(ord('a') + lang_code // (27 * 27) - 1)
        region_code = (low // (27 * 27 * 27)) % (27 * 27)
        if region_code < 27:
            region = m49[region_code]
        else:
            region = chr(ord('A') + region_code % 27 - 1) + chr(ord('A') + (region_code // 27) % 27 - 1)
        script = code_to_script.get((v >> 24) & 0xff)
        if script is None:
            raise SystemExit(f'unnamed script code {(v >> 24) & 0xff}')
        return [lang, script, region]

    return {'languageAliases': dict(zip(info['languageAliases'][::2], info['languageAliases'][1::2])),
            'regionAliases': dict(zip(info['regionAliases'][::2], info['regionAliases'][1::2])),
            'm49': m49, 'trie': tree, 'lsrs': [lsr(v) for v in nums]}


def strip_samples(rules: dict) -> dict:
    return {name: {cat: re.split(r'\s*@', text, maxsplit=1)[0].strip() for cat, text in table.items()} for name, table in rules.items()}


AVAILABLE = r"""
const tags = JSON.parse(process.argv[1]);
const out = {};
for (const [k, c] of [['nf', Intl.NumberFormat], ['dtf', Intl.DateTimeFormat], ['pr', Intl.PluralRules]]) {
  out[k] = tags.filter(t => { try { return c.supportedLocalesOf([t]).length === 1; } catch { return false; } });
}
console.log(JSON.stringify(out));
"""


def available(names: list[str]) -> dict:
    tags = set()
    for name in names:
        parts = name.split('_')
        tags.add('-'.join(parts))
        if len(parts) == 3 and len(parts[1]) == 4:
            tags.add(parts[0] + '-' + parts[2])
    return json.loads(node(AVAILABLE, json.dumps(sorted(tags))))


CALENDAR_SAMPLES = r"""
const DAY = 86400000;
const fields = (cal, ms) => {
  const f = fields[cal] ||= new Intl.DateTimeFormat(`en-US-u-ca-${cal}-nu-latn`, {timeZone: 'UTC', year: 'numeric', month: 'numeric', day: 'numeric'});
  const o = {};
  for (const p of f.formatToParts(ms)) if (p.type !== 'literal') o[p.type] = p.value;
  return o;
};
// First day of every Persian year -2000..4000.
const starts = [];
for (let y = -2000; y <= 4000; y++) {
  let lo = Math.floor((y - 1349) * 365.2422) - 400, hi = lo + 800;
  while (hi - lo > 1) {
    const mid = Math.floor((lo + hi) / 2);
    if (Number(fields('persian', mid * DAY).year) >= y) hi = mid; else lo = mid;
  }
  starts.push(hi);
}
// Umm al-Qura month starts 1850..2200: day, year, zero-based month.
const uq = [];
let prev = null;
const from = Date.UTC(1850, 0, 1) / DAY, to = Date.UTC(2200, 0, 1) / DAY;
for (let d = from; d < to; d++) {
  const r = fields('islamic-umalqura', d * DAY);
  const key = r.year + '|' + r.month;
  if (key !== prev) { uq.push([d, parseInt(r.year, 10), parseInt(r.month, 10) - 1]); prev = key; }
}
console.log(JSON.stringify({persian: {first: -2000, starts}, umalqura: uq}));
"""


def calendar_data(sd: dict) -> dict:
    eras = sd['calendarData']['japanese']['eras']
    out = json.loads(node(CALENDAR_SAMPLES))
    out['japaneseEras'] = [eras[str(i)]['start'] for i in range(len(eras))]
    return out


def main() -> int:
    pkg = Package(sys.argv[1])
    with zipfile.ZipFile(sys.argv[2], 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        def put(name: str, value) -> None:
            info = zipfile.ZipInfo(name, date_time=(2026, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            z.writestr(info, dumps(value))

        names = locale_names(pkg, '')
        for name in names:
            put('main/' + name, trim_main(pkg.bundle(name + '.res').read()))
        for name in locale_names(pkg, 'curr/'):
            res = pkg.bundle('curr/' + name + '.res').read()
            put('curr/' + name, {k: v for k, v in res.items() if k != 'Version' and k != 'Currencies%variant'})
        for name in locale_names(pkg, 'unit/'):
            res = pkg.bundle('unit/' + name + '.res').read()
            put('unit/' + name, trim_units({k: v for k, v in res.items() if k != 'Version'}))
        for name in locale_names(pkg, 'zone/'):
            res = pkg.bundle('zone/' + name + '.res').read()
            put('zone/' + name, {k: v for k, v in res.items() if k != 'Version'})

        for name in locale_names(pkg, 'region/'):
            res = pkg.bundle('region/' + name + '.res').read()
            kept = {k: res[k] for k in ('Countries', '%%Parent', '%%ALIAS') if k in res}
            put('region/' + name, kept)

        plurals = pkg.bundle('plurals.res').read()
        put('supp/plurals', {'locales': plurals['locales'], 'ordinals': plurals['locales_ordinals'],
                             'rules': strip_samples(plurals['rules'])})
        put('supp/numberingSystems', {k: {'digits': v['desc'], 'algorithmic': v['algorithmic']}
                                      for k, v in pkg.bundle('numberingSystems.res').read()['numberingSystems'].items()})
        day = pkg.bundle('dayPeriods.res').read()
        put('supp/dayPeriods', {'locales': day['locales'], 'rules': day['rules']})
        meta = pkg.bundle('metadata.res').read()['alias']
        put('supp/aliases', {k: {a: b['replacement'] for a, b in meta[k].items() if 'replacement' in b}
                             for k in ('language', 'script', 'territory', 'variant', 'subdivision')})
        sd = pkg.bundle('supplementalData.res').read()
        put('supp/time', {'timeData': sd['timeData'], 'weekData': sd['weekData']})
        put('supp/likely', likely(pkg))
        currency_data = pkg.bundle('curr/supplementalData.res').read()
        put('supp/currencyMeta', currency_data['CurrencyMeta'])
        # The legal tender in force for each region, the first entry ICU's ucurr_forLocale accepts.
        put('supp/currencyMap', {region: next((e['id'] for e in entries_ if e.get('tender') != 'false'), None)
                                 for region, entries_ in currency_data['CurrencyMap'].items()})
        mz = pkg.bundle('metaZones.res').read()
        put('supp/metaZones', {'info': mz['metazoneInfo'], 'map': mz['mapTimezones'], 'primary': mz['primaryZones']})
        tt = pkg.bundle('timezoneTypes.res').read()
        put('supp/zoneAliases', {'alias': tt['typeAlias']['timezone'], 'iana': tt['ianaMap']['timezone'],
                                 'canonical': sorted(tt['typeMap']['timezone'])})
        put('supp/zoneinfo', pkg.bundle('zoneinfo64.res').read())
        kt = pkg.bundle('keyTypeData.res').read()
        put('supp/keyTypes', {'calendar': kt['typeMap'].get('calendar', {}), 'calendarAlias': kt['typeAlias'].get('calendar', {}),
                              'caBcpAlias': kt['bcpTypeAlias'].get('ca', {})})
        put('supp/calendar', calendar_data(sd))
        put('supp/defaultScripts', default_scripts(open(sys.argv[3], 'rb').read()))
        idx = pkg.bundle('res_index.res').read()
        put('supp/index', {'installed': sorted(idx['InstalledLocales']), 'alias': sorted(idx['AliasLocales']),
                           'available': available(sorted(set(names) | set(idx['InstalledLocales']) | set(idx['AliasLocales'])))})
    return 0


if __name__ == '__main__':
    sys.exit(main())
