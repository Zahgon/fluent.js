"""Find ICU's compiled default-script table (localefallback_data.h) inside a node binary."""
from __future__ import annotations

import struct


def default_scripts(binary: bytes) -> dict[str, str]:
    """{language or language_REGION: script} for every locale whose default script is not Latn."""
    anchor = binary.find(b'\0aaf\0aao\0aat\0ab\0abh\0')
    if anchor < 0:
        raise SystemExit('dsLocaleIDChars not found')
    start = anchor + 1
    # Offsets into dsLocaleIDChars ascend; find the table of int32 pairs whose locale offsets step as the names do.
    names_at = {}
    pos = start
    while True:
        end = binary.index(b'\0', pos)
        if end == pos:
            break
        name = binary[pos:end]
        if not all(32 < c < 127 for c in name):
            break
        names_at[pos - start] = name.decode()
        pos = end + 1
    steps = [4, 4, 3, 4]  # aaf aao aat ab abh
    align = 4
    words_needed = 12
    window = 40 * 1024 * 1024
    lo = max(0, anchor - window) & ~3
    hi = min(len(binary), anchor + window)
    table_at = None
    for i in range(lo, hi - 4 * words_needed, align):
        a0 = struct.unpack_from('<i', binary, i)[0]
        if a0 != 4:
            continue
        a = struct.unpack_from('<10i', binary, i)
        if a[2] - a[0] == steps[0] and a[4] - a[2] == steps[1] and a[6] - a[4] == steps[2] and a[8] - a[6] == steps[3]:
            table_at = i
            break
    if table_at is None:
        raise SystemExit('defaultScriptTable not found')
    # The first match is the entry of aao, offset 4 from aaf; step back to aaf at offset 0.
    table_at -= 8
    base = start
    table: dict[str, str] = {}
    pairs = []
    i = table_at
    previous = -1
    while True:
        loc, scr = struct.unpack_from('<2i', binary, i)
        off = base + loc
        if loc <= previous or (off - start) not in names_at or not 0 <= scr < 4096:
            break
        previous = loc
        pairs.append((names_at[off - start], scr))
        i += 8
    # Identify the script offsets by reading scriptCodeChars around the locale run.
    code_base = None
    known = {'ab': 'Cyrl', 'am': 'Ethi', 'ar': 'Arab', 'hi': 'Deva', 'ja': 'Jpan', 'ko': 'Kore'}
    wanted = {name: off for name, off in pairs if name in known}
    for cand in range(max(0, anchor - (1 << 18)), anchor + (1 << 18)):
        if all(binary[cand + off:cand + off + 4] == known[name].encode() for name, off in wanted.items()):
            code_base = cand
            break
    if code_base is None:
        raise SystemExit('scriptCodeChars not found')
    for name, off in pairs:
        code = binary[code_base + off:code_base + off + 4]
        if not code.isalpha():
            break
        table[name] = code.decode()
    return table
