FROM golang:1.20-alpine AS backend
WORKDIR /go/src/kubelab-agent
COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./pkg ./pkg
COPY ./go.mod .
COPY ./go.sum .
ENV CGO_ENABLED=0
RUN go mod vendor
ARG VERSION_INFO=dev-build
RUN go build -a -v \
  -ldflags " \
  -s -w \
  -extldflags 'static' \
  -X main.VersionInfo='${VERSION_INFO}' \
  " \
  -o ./bin/kubelab-agent \
  ./cmd/kubelab-agent

FROM ubuntu:22.04
WORKDIR /app
# Adding base utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates \
  bash \
  wget \
  curl \
  openssl \
  jq \
  vim \
  nano \
  dnsutils \
  openssh-client \
  build-essential \
  libffi-dev \
  libssl-dev \
  bash-completion \
  podman && \
  rm -rf /var/lib/apt/lists/*

# Installing kubectl
RUN curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
  chmod +x ./kubectl && \
  mv ./kubectl /usr/local/bin/kubectl

# Installing helm
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
  chmod +x ./get_helm.sh && \
  ./get_helm.sh && \
  rm ./get_helm.sh

# Installing kustomize
RUN curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash && \
  mv ./kustomize /usr/local/bin/kustomize

# Installing cri-o
RUN curl https://raw.githubusercontent.com/cri-o/cri-o/main/scripts/get | bash

COPY --from=backend /go/src/kubelab-agent/bin/kubelab-agent /app/kubelab-agent
RUN ln -s /app/kubelab-agent /usr/bin/kubelab-agent

RUN groupadd -g 1001 kubelab-agent
RUN useradd -s /bin/bash -u 1001 -g 1001 -m kubelab-agent

RUN mkdir -p /home/kubelab-agent/.kube

# Autocompletion for kubectl
RUN echo 'source <(kubectl completion bash)' >>/home/kubelab-agent/.bashrc
RUN echo 'alias k=kubectl' >>/home/kubelab-agent/.bashrc
# Autocompletion for helm
RUN echo 'source <(helm completion bash)' >>/home/kubelab-agent/.bashrc
# Autocompletion for kustomize
RUN echo 'source <(kustomize completion bash)' >>/home/kubelab-agent/.bashrc
# Autocompletion for podman
RUN echo 'source <(podman completion bash)' >>/home/kubelab-agent/.bashrc
# Autocompletion for crio
RUN echo 'source <(crio completion bash)' >>/home/kubelab-agent/.bashrc

RUN chown kubelab-agent:kubelab-agent /app -R
RUN chown kubelab-agent:kubelab-agent /home/kubelab-agent -R
WORKDIR /home/kubelab-agent

COPY ./assets/.vimrc /home/kubelab-agent/.vimrc
RUN chown kubelab-agent:kubelab-agent /home/kubelab-agent/.vimrc

# add export TERM=xterm
RUN echo 'export TERM=xterm' >>/home/kubelab-agent/.bashrc

# replace existing PS1 with a shorter to username@kubelab-agent and current working directory
RUN echo 'export PS1="\[\033[01;34m\]\u@kubelab-agent\[\033[00m\]:\[\033[01;34m\]\w\[\033[00m\]\\$ "' >>/home/kubelab-agent/.bashrc

ENV WORKDIR=/app
USER kubelab-agent
ENTRYPOINT ["/app/kubelab-agent"]
