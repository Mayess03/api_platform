FROM kong/kong-gateway:3.9

USER root

RUN apt-get update \
    && apt-get install -y \
        unzip \
        git \
        gcc \
        make \
        zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

RUN luarocks install --server=http://luarocks.org/manifests/moesif kong-plugin-moesif

USER kong