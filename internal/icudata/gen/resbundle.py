#!/usr/bin/env python3
"""Read ICU resource bundles (format 2 and 3) out of an ICU common data package (icudt*.dat)."""
from __future__ import annotations

import struct

BASE_TYPE = {0: 'string', 1: 'binary', 2: 'table', 3: 'alias', 4: 'table32', 5: 'table16', 6: 'string_v2', 7: 'int',
             8: 'array', 9: 'array16', 14: 'intvector'}


class Package:
    def __init__(self, path: str):
        self.data = open(path, 'rb').read()
        hdr = struct.unpack_from('<H', self.data, 0)[0]
        self.base = hdr
        count = struct.unpack_from('<I', self.data, hdr)[0]
        self.items: dict[str, tuple[int, int]] = {}
        for i in range(count):
            name_off, data_off = struct.unpack_from('<II', self.data, hdr + 4 + i * 8)
            end = self.data.index(b'\0', hdr + name_off)
            name = self.data[hdr + name_off:end].decode()
            self.items[name.split('/', 1)[1]] = (hdr + data_off, 0)
        self._bundles: dict[str, Bundle] = {}

    def has(self, name: str) -> bool:
        return name in self.items

    def bundle(self, name: str) -> 'Bundle':
        b = self._bundles.get(name)
        if b is None:
            b = Bundle(self, name)
            self._bundles[name] = b
        return b


class Bundle:
    def __init__(self, pkg: Package, name: str):
        self.pkg = pkg
        self.name = name
        d = pkg.data
        start = pkg.items[name][0]
        hsize = struct.unpack_from('<H', d, start)[0]
        self.d = d
        self.root = start + hsize
        i32 = lambda i: struct.unpack_from('<i', d, self.root + i * 4)[0]
        self.root_res = struct.unpack_from('<I', d, self.root)[0]
        word1 = struct.unpack_from('<I', d, self.root + 4)[0]
        index_len = word1 & 0xff
        keys_top = i32(2)  # URES_INDEX_KEYS_TOP
        self.local_key_limit = keys_top << 2
        self.p16 = None
        self.pool_limit = word1 >> 8
        self.pool16_limit = 0
        self.index_len = index_len
        self.is_pool = False
        self.uses_pool = False
        if index_len > 5:
            att = i32(1 + 5)
            self.is_pool = bool(att & 2)
            self.uses_pool = bool(att & 4)
            self.pool_limit |= (att & 0xf000) << 12
            self.pool16_limit = (att & 0xffffffff) >> 16
        if index_len > 6:
            top16 = i32(1 + 6)
            if top16 > keys_top:
                self.p16 = self.root + keys_top * 4
        self.keys_base = self.root + 4 * (1 + index_len)
        if keys_top == 1 + index_len:
            self.local_key_limit = 0  # no local keys: key offsets are direct offsets into the pool's keys
        self.pool = None
        if self.uses_pool:
            prefix = name.rsplit('/', 1)[0] + '/' if '/' in name else ''
            self.pool = pkg.bundle(prefix + 'pool.res')

    def key16(self, off: int) -> str:
        if off < self.local_key_limit:
            return self._cstr(self.root + off)
        return self._cstr(self.pool.keys_base + off - self.local_key_limit)

    def key32(self, off: int) -> str:
        if off >= 0:
            return self._cstr(self.root + off)
        return self._cstr(self.pool.keys_base + (off & 0x7fffffff))

    def _cstr(self, p: int) -> str:
        e = self.d.index(b'\0', p)
        return self.d[p:e].decode('ascii')

    def _u16(self, p: int) -> int:
        return struct.unpack_from('<H', self.d, p)[0]

    def _str_at(self, p: int, length: int) -> str:
        return self.d[p:p + length * 2].decode('utf-16-le', 'surrogatepass')

    def string_v2(self, off: int) -> str:
        if off < self.pool_limit:
            base = self.pool.p16 + off * 2
        else:
            base = self.p16 + (off - self.pool_limit) * 2
        first = self._u16(base)
        if not 0xdc00 <= first <= 0xdfff:
            p = base
            while self._u16(p):
                p += 2
            return self._str_at(base, (p - base) // 2)
        if first < 0xdfef:
            return self._str_at(base + 2, first & 0x3ff)
        if first < 0xdfff:
            return self._str_at(base + 4, ((first - 0xdfef) << 16) | self._u16(base + 2))
        return self._str_at(base + 6, (self._u16(base + 2) << 16) | self._u16(base + 4))

    def from16(self, r: int) -> int:
        if r < self.pool16_limit:
            return (6 << 28) | r
        return (6 << 28) | (r - self.pool16_limit + self.pool_limit)

    def read(self, res: int | None = None):
        if res is None:
            res = self.root_res
        typ = res >> 28
        off = res & 0x0fffffff
        d = self.d
        if typ == 6:
            return self.string_v2(off)
        if typ == 0:
            if off == 0:
                return ''
            p = self.root + off * 4
            n = struct.unpack_from('<i', d, p)[0]
            return self._str_at(p + 4, n)
        if typ == 3:
            p = self.root + off * 4
            n = struct.unpack_from('<i', d, p)[0]
            return {'$alias': self._str_at(p + 4, n)}
        if typ == 7:
            return off - (1 << 28) if off & (1 << 27) else off
        if typ == 14:
            if off == 0:
                return []
            p = self.root + off * 4
            n = struct.unpack_from('<i', d, p)[0]
            return list(struct.unpack_from(f'<{n}i', d, p + 4))
        if typ == 1:
            if off == 0:
                return {'$binary': ''}
            p = self.root + off * 4
            n = struct.unpack_from('<i', d, p)[0]
            return {'$binary': d[p + 4:p + 4 + n].hex()}
        if typ == 2:
            if off == 0:
                return {}
            p = self.root + off * 4
            n = self._u16(p)
            keys = struct.unpack_from(f'<{n}H', d, p + 2)
            q = p + 2 + n * 2
            if (n + 1) % 2:
                q += 2
            items = struct.unpack_from(f'<{n}I', d, q)
            return {self.key16(k): self.read(v) for k, v in zip(keys, items)}
        if typ == 5:
            p = self.p16 + off * 2
            n = self._u16(p)
            keys = struct.unpack_from(f'<{n}H', d, p + 2)
            items = struct.unpack_from(f'<{n}H', d, p + 2 + n * 2)
            return {self.key16(k): self.read(self.from16(v)) for k, v in zip(keys, items)}
        if typ == 4:
            if off == 0:
                return {}
            p = self.root + off * 4
            n = struct.unpack_from('<i', d, p)[0]
            keys = struct.unpack_from(f'<{n}i', d, p + 4)
            items = struct.unpack_from(f'<{n}I', d, p + 4 + n * 4)
            return {self.key32(k): self.read(v) for k, v in zip(keys, items)}
        if typ == 8:
            if off == 0:
                return []
            p = self.root + off * 4
            n = struct.unpack_from('<i', d, p)[0]
            items = struct.unpack_from(f'<{n}I', d, p + 4)
            return [self.read(v) for v in items]
        if typ == 9:
            p = self.p16 + off * 2
            n = self._u16(p)
            items = struct.unpack_from(f'<{n}H', d, p + 2)
            return [self.read(self.from16(v)) for v in items]
        raise ValueError(f'resource type {typ} in {self.name}')
