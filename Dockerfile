FROM golang:1.14.2
RUN \
  apt-get update -qq && \
  apt-get install -qq -y build-essential autoconf automake autotools-dev
