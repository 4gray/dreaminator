#!/bin/sh
# Virtual receiver: fake Enigma2/OpenWebif/EPGImport on :80 (+ TV page on :8090)
# and the real mipsel Dreaminator binary from the .ipk, run on an emulated
# MIPS32R1 CPU without FPU (-cpu 4Kc), the lowest common denominator for the
# DM520's BMIPS4380.
fakebox -webif :80 -ui :8090 &
echo "Dreaminator (MIPS через QEMU):  http://localhost:8088"
echo "Виртуальный телевизор:         http://localhost:8090"
exec qemu-mipsel-static -cpu 4Kc /usr/bin/dreaminator -listen :8088 -data /etc/dreaminator
