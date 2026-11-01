#!/usr/bin/env python3
"""Locate the ICU common data package (dataFormat CmnD) in a node binary and write it out.

Usage: finddat.py NODE_BINARY OUT.dat
"""
import struct
import sys

blob = open(sys.argv[1], 'rb').read()
pos = 0
found = []
while True:
    pos = blob.find(b'CmnD', pos)
    if pos < 0:
        break
    start = pos - 12  # UDataInfo.dataFormat sits at offset 12 of the header (after size, magic, info size...)
    if start >= 0 and blob[start + 2] == 0xda and blob[start + 3] == 0x27:
        found.append(start)
    pos += 4
if not found:
    sys.exit('no CmnD package found')
start = found[0]
hdr = struct.unpack_from('<H', blob, start)[0]
count = struct.unpack_from('<I', blob, start + hdr)[0]
entries = [struct.unpack_from('<II', blob, start + hdr + 4 + i * 8) for i in range(count)]
last_data = entries[-1][1]
# The final item's size is not in the table; read it from its own header and content is bounded below.
end = start + hdr + last_data + 4096
first_name = blob[start + hdr + entries[0][0]:start + hdr + entries[0][0] + 40].split(b'\0')[0]
last_name = blob[start + hdr + entries[-1][0]:start + hdr + entries[-1][0] + 40].split(b'\0')[0]
print(f'offset {start} header {hdr} items {count} first {first_name} last {last_name}')
open(sys.argv[2], 'wb').write(blob[start:end])
print('wrote', end - start)
