"""Generate the hook signatures used by internal/hooks/hooks.go.

Give it a NewWorld.exe and the virtual addresses of the three functions in THAT
exe; it prints a relocation-proof signature for each (rip-relative displacements
and relative call/branch targets become "??") and proves the signature is unique
in the exe's .text.

    pip install capstone
    python tools/gen_sigs.py --exe NewWorld.exe --read 0x1478F2E20 --write 0x1478F3460 --keylog 0x1478F39E0

When a game patch changes these functions, find the new addresses (see
docs/HOOKS.md), run this, and paste the output into hooks.go.
"""
import argparse
import struct
import sys

try:
    import capstone
    from capstone import x86
except ImportError:
    sys.exit("needs capstone: pip install capstone")

MIN_LEN = 64


def load_pe(path):
    data = open(path, "rb").read()
    e = struct.unpack_from("<I", data, 0x3C)[0]
    nsec = struct.unpack_from("<H", data, e + 6)[0]
    osz = struct.unpack_from("<H", data, e + 20)[0]
    opt = e + 24
    base = struct.unpack_from("<Q", data, opt + 24)[0]
    secs = []
    for i in range(nsec):
        name, vsize, vaddr, rawsize, rawptr = struct.unpack_from("<8sIIII", data, opt + osz + i * 40)
        secs.append((name.rstrip(b"\0").decode(), vaddr, rawptr, rawsize))
    return data, base, secs


def make_sig(code):
    md = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64)
    md.detail = True
    mask = bytearray(len(code))  # 1 = wildcard
    end = 0
    for insn in md.disasm(code, 0):
        end = insn.address + insn.size
        for op in insn.operands:
            if op.type == x86.X86_OP_MEM and op.mem.base == x86.X86_REG_RIP:
                for j in range(insn.disp_offset, insn.disp_offset + insn.disp_size):
                    mask[insn.address + j] = 1
        if insn.group(capstone.CS_GRP_JUMP) or insn.group(capstone.CS_GRP_CALL):
            for op in insn.operands:
                if op.type == x86.X86_OP_IMM and insn.imm_size in (1, 4):
                    for j in range(insn.imm_offset, insn.imm_offset + insn.imm_size):
                        mask[insn.address + j] = 1
        if end >= MIN_LEN:
            break
    return code[:end], mask[:end]


def find_all(buf, code, mask):
    k = 0
    while mask[k]:
        k += 1
    j = k
    while j < len(mask) and not mask[j]:
        j += 1
    anchor, hits, start = bytes(code[k:j]), [], 0
    while True:
        p = buf.find(anchor, start)
        if p < 0:
            return hits
        s = p - k
        if s >= 0 and s + len(code) <= len(buf) and all(m or buf[s + i] == code[i] for i, m in enumerate(mask)):
            hits.append(s)
        start = p + 1


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--exe", required=True)
    for n in ("read", "write", "keylog"):
        ap.add_argument("--" + n, required=True, type=lambda v: int(v, 16))
    a = ap.parse_args()

    data, base, secs = load_pe(a.exe)
    text = next(s for s in secs if s[0] == ".text")
    _, tva, traw, tsize = text
    tdata = data[traw:traw + tsize]

    bad = False
    for name, va in (("sigSSLRead", a.read), ("sigSSLWrite", a.write), ("sigKeylog", a.keylog)):
        off = va - base - tva
        code, mask = make_sig(tdata[off:off + 160])
        hits = find_all(tdata, code, mask)
        unique = hits == [off]
        bad |= not unique
        sig = " ".join("??" if m else "%02X" % b for b, m in zip(code, mask))
        print("// %s: %d bytes, %d wildcards, %s" % (name, len(code), sum(mask),
              "unique in .text" if unique else "NOT UNIQUE: %d matches" % len(hits)))
        print('%s = "%s"\n' % (name, sig))
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
