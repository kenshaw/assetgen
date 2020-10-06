FROM golang:1.15.2
RUN \
  apt-get update -qq && \
  apt-get install -qq -y build-essential autoconf automake autotools-dev
