FROM golang:1.16.0
RUN \
  apt-get update -qq && \
  apt-get install -qq -y build-essential autoconf automake autotools-dev
