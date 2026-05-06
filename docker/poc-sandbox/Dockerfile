FROM golang:1.25-bookworm

RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		bash \
		ca-certificates \
		coreutils \
		curl \
		findutils \
		gawk \
		git \
		grep \
		jq \
		make \
		nodejs \
		npm \
		python3 \
		ripgrep \
		sed \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

CMD ["/bin/bash"]
