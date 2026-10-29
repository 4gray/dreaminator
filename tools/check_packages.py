#!/usr/bin/env python3
"""Verify release IPKs contain the intended CPU, version and current artwork."""
import io
from pathlib import Path
import struct
import tarfile

ROOT = Path(__file__).resolve().parents[1]
VERSION = next(line.split('=', 1)[1].strip() for line in (ROOT / 'Makefile').read_text().splitlines()
               if line.startswith('VERSION ?='))


def members(path):
    data = path.read_bytes()
    assert data[:8] == b'!<arch>\n', 'not an ar archive'
    offset, result = 8, {}
    while offset < len(data):
        header = data[offset:offset + 60]
        assert len(header) == 60 and header[58:60] == b'`\n'
        name = header[:16].decode().strip().rstrip('/')
        size = int(header[48:58])
        offset += 60
        result[name] = data[offset:offset + size]
        offset += size + size % 2
    assert offset == len(data)
    return result


def files(data):
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:gz') as archive:
        return {m.name.removeprefix('./'): archive.extractfile(m).read()
                for m in archive if m.isfile()}


def check(arch, machine):
    path = ROOT / 'dist' / ('dreaminator_%s_%s.ipk' % (VERSION, arch))
    ar = members(path)
    assert ar['debian-binary'] == b'2.0\n'
    control = files(ar['control.tar.gz'])['control'].decode()
    assert '\nVersion: %s\n' % VERSION in control
    assert '\nArchitecture: %s\n' % arch in control
    data = files(ar['data.tar.gz'])
    binary = data['usr/bin/dreaminator']
    assert binary[:6] == b'\x7fELF\x01\x01', 'expected little-endian ELF32'
    assert struct.unpack_from('<H', binary, 18)[0] == machine, 'wrong CPU'
    for name in ['LICENSE', 'THIRD_PARTY_NOTICES.md']:
        assert data['usr/share/doc/dreaminator/' + name] == (ROOT / name).read_bytes()
    # Compare every shipped plugin file, including branding, fonts and EPG entry.
    for source in sorted((ROOT / 'plugin').rglob('*')):
        if not source.is_file() or '__pycache__' in source.parts or source.suffix in ('.pyc', '.pyo') or source.name.startswith('.'):
            continue
        dest = 'usr/lib/enigma2/python/Plugins/Extensions/' + source.relative_to(ROOT / 'plugin').as_posix()
        assert data[dest] == source.read_bytes(), 'stale/missing plugin asset: ' + dest
    print('%s: version, CPU and all plugin assets OK' % path.name)


if __name__ == '__main__':
    check('all', 8)           # EM_MIPS
    check('vuuno4kse', 40)   # EM_ARM
