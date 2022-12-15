# syntax=docker/dockerfile:1.3
FROM alpine:3.17
LABEL org.opencontainers.image.authors="Wrangle Ltd <khoi@wrgl.co>"
LABEL org.opencontainers.image.source="https://github.com/wrgl/wrgld"
LABEL org.opencontainers.image.licenses="Apache-2.0+Common-Clause"
LABEL org.opencontainers.image.title="Wrgld"

ARG USER=wrgld
ENV HOME /home/$USER
RUN adduser -D $USER

COPY LICENSE /usr/local/share/doc/wrgl/
COPY bin/wrgld /usr/local/bin/wrgld
RUN mkdir -p /var/lib/wrgl/data

USER $USER
WORKDIR /var/lib/wrgl/
ENTRYPOINT ["/usr/local/bin/wrgld", "/var/lib/wrgl/data"]
