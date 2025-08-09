FROM golang:1.24.2 AS base
ARG PROXY
WORKDIR /app
RUN sed -i 's/deb.debian.org/mirrors.aliyun.com/g' \
    /etc/apt/sources.list.d/debian.sources && \
    apt update && \
    apt upgrade -y && \
    apt install zip unzip -y && \
    export https_proxy=${PROXY} && \
    git clone https://github.com/magefile/mage.git && \
    cd mage && \
    export PATH=$PATH:/usr/local/go/bin && \
    go run bootstrap.go && \ 
    wget -qO- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash && \
    echo 'export NVM_DIR="$HOME/.nvm"' >> /root/.bashrc && \
    echo '[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"' >> /root/.bashrc && \
    echo '[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"' >> /root/.bashrc && \
    . /root/.bashrc && \
    nvm install 22 && \
    nvm use 22 && \
    nvm alias default 22

FROM base AS builder
ARG ARCH
COPY . .
ENV GOPROXY=https://goproxy.cn,direct
RUN  if [ ${ARCH} = "amd64" ]; then \
    mage -v build:linux; \
    elif [ ${ARCH} = "arm64" ]; then \
    mage -v build:linuxarm64; \
    else \
    echo "Unsupported architecture: ${ARCH}"; exit 1; \
    fi
RUN . /root/.bashrc && \
    nvm use 22 && npm ci && npm run build && \
    cp -r dist openobserve-grafana-plugin-${ARCH} && \
    zip openobserve-grafana-plugin-${ARCH}.zip openobserve-grafana-plugin-${ARCH} -r

FROM busybox:1.28 AS runtime
COPY --from=builder /app/dist openobserve-grafana-plugin