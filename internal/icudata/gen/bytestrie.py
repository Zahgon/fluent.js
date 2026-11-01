"""Enumerate an ICU BytesTrie into (key bytes, value, is_final) entries."""
from __future__ import annotations


def _read_value(d, pos, lead):
    if lead < 0x51:
        return lead - 0x10, pos
    if lead < 0x6c:
        return ((lead - 0x51) << 8) | d[pos], pos + 1
    if lead < 0x7e:
        return ((lead - 0x6c) << 16) | (d[pos] << 8) | d[pos + 1], pos + 2
    if lead == 0x7e:
        return (d[pos] << 16) | (d[pos + 1] << 8) | d[pos + 2], pos + 3
    return (d[pos] << 24) | (d[pos + 1] << 16) | (d[pos + 2] << 8) | d[pos + 3], pos + 4


def _read_delta(d, pos):
    v = d[pos]
    pos += 1
    if v >= 0xc0:
        if v < 0xf0:
            v = ((v - 0xc0) << 8) | d[pos]
            pos += 1
        elif v < 0xfe:
            v = ((v - 0xf0) << 16) | (d[pos] << 8) | d[pos + 1]
            pos += 2
        elif v == 0xfe:
            v = (d[pos] << 16) | (d[pos + 1] << 8) | d[pos + 2]
            pos += 3
        else:
            v = (d[pos] << 24) | (d[pos + 1] << 16) | (d[pos + 2] << 8) | d[pos + 3]
            pos += 4
    return v, pos


def entries(d: bytes):
    out: list[tuple[bytes, int, bool]] = []

    def node(pos: int, key: bytes):
        while True:
            lead = d[pos]
            pos += 1
            if lead >= 0x20:
                final = bool(lead & 1)
                val, pos = _read_value(d, pos, lead >> 1)
                out.append((key, val, final))
                if final:
                    return
            elif lead >= 0x10:
                n = lead - 0x10 + 1
                key += d[pos:pos + n]
                pos += n
            else:
                length = lead
                if length == 0:
                    length = d[pos]
                    pos += 1
                branch(pos, length + 1, key)
                return

    def branch(pos: int, length: int, key: bytes):
        while length > 5:
            pos += 1  # the comparison byte: both halves are enumerated
            delta, after = _read_delta(d, pos)
            branch(after + delta, length >> 1, key)
            pos = after
            length = length - (length >> 1)
        while length > 1:
            byte = d[pos]
            pos += 1
            lead = d[pos]
            pos += 1
            final = bool(lead & 1)
            val, pos = _read_value(d, pos, lead >> 1)
            if final:
                out.append((key + bytes([byte]), val, True))
            else:
                node(pos + val, key + bytes([byte]))
            length -= 1
        byte = d[pos]
        pos += 1
        node(pos, key + bytes([byte]))

    node(0, b'')
    return out
