# Build context: repository root (see `make sim`).
# No ca-certificates on purpose: like the old receiver image, Dreaminator must
# cope with its bundled root certificates.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends qemu-user-static python3 \
 && rm -rf /var/lib/apt/lists/*
COPY sim/rootfs/ /
COPY dist/dreaminator.ipk /tmp/dreaminator.ipk
# dpkg-deb reads the same ar/tar.gz layout opkg does: this checks the package.
RUN dpkg-deb --info /tmp/dreaminator.ipk \
 && dpkg-deb --extract /tmp/dreaminator.ipk / \
 && rm /tmp/dreaminator.ipk
COPY dist/fakebox /usr/local/bin/fakebox
COPY sim/run.sh /run.sh
EXPOSE 8088 8090
CMD ["/run.sh"]
