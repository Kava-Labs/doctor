# Use official golang image as base
# OS for building doctor and kava binaries
ARG go_builder_image=golang:1.18.3
FROM $go_builder_image as go-builder

# Install packages needed to build the golang binaries
RUN apt-get update \
      && apt-get install -y git make gcc \
      && rm -rf /var/lib/apt/lists/*

RUN mkdir /app
WORKDIR /app

# Download and build kava from remote sources
ARG kava_node_version=v0.17.5
ENV KAVA_NODE_VERSION=$kava_node_version

RUN git clone https://github.com/kava-labs/kava \
      && cd kava \
      && git checkout $KAVA_NODE_VERSION \
      && make install

# Copy over local sources
# and build the doctor binary
RUN mkdir /app/doctor
WORKDIR /app/doctor

COPY Makefile ./

COPY *.go go.mod go.sum ./

COPY .env ./

RUN make install

# Copy over configuration and scripts
# for running kava and doctor services
COPY docker/ ./

# Use a mimimial production like base
# image for running kava and doctor services
FROM ubuntu:jammy

RUN apt-get update \
      && apt-get install -y supervisor curl \
      && rm -rf /var/lib/apt/lists/*

RUN mkdir /app \
      && mkdir /app/bin
WORKDIR /app

# update path for docker user to include
# kava and doctor binaries
ENV PATH=$PATH:/app/bin

# copy build binaries from build environment
COPY --from=go-builder /go/bin/kava /app/bin/kava
COPY --from=go-builder /go/bin/doctor /app/bin/doctor

# copy config templates to automate setup
COPY --from=go-builder /app/doctor/chain-configs /app/templates

# copy scripts to run services
COPY --from=go-builder /app/doctor/supervisord/start-services.sh /app/bin/start-services.sh
COPY --from=go-builder /app/doctor/supervisord/kill-supervisord.sh /app/bin/kill-supervisord.sh
COPY --from=go-builder /app/doctor/supervisord/supervisord.conf /etc/supervisor/conf.d/doctor.conf

# by default start kava and doctor services
# using the configuration in /app/templates
CMD ["/app/bin/start-services.sh"]
