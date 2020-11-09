FROM golang:1.15.4
RUN \
  apt-get update -qq && \
  apt-get install -qq -y build-essential autoconf automake autotools-dev
