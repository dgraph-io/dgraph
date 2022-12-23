FROM ubuntu:20.04
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"
RUN rm -rf /var/lib/apt/lists/*
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update -qq && apt-get install -y nfs-kernel-server runit inotify-tools -qq
RUN mkdir -p /dgraph-data
RUN apt-get install -y --no-install-recommends ca-certificates="20211016ubuntu0.20.04.1"
RUN apt-get install -y --no-install-recommends curl="7.68.0-1ubuntu2.14"
RUN apt-get install -y --no-install-recommends htop="2.2.0-2build1"
RUN apt-get install -y --no-install-recommends iputils-ping="3:20190709-3"
RUN apt-get install -y --no-install-recommends jq="1.6-1ubuntu0.20.04.1"
RUN apt-get install -y --no-install-recommends less="551-1ubuntu0.1"
RUN apt-get install -y --no-install-recommends sysstat="12.2.0-2ubuntu0.2"

COPY setups/nfs-common /etc/default/nfs-common
COPY setups/nfs-kernel-server /etc/default/nfs-kernel-server
COPY setups/quota /etc/default/quota
COPY setups/nfs-static-ports.conf /etc/sysctl.d/nfs-static-ports.conf
COPY setups/nfs-static-ports.conf /etc/sysctl.conf
COPY setups/nfs /etc/sysconfig/nfs
ADD linux  /usr/local/bin
RUN mkdir -p /etc/sv/rpc_bind
ADD rpcbind.init /etc/sv/rpc_bind/run

ADD rpcbind.stop /etc/sv/rpc_bind/finish
ADD rundgraph.sh  /usr/local/bin/rundgraph.sh
RUN chmod u+x /usr/local/bin/rundgraph.sh
ADD rpc_setup.sh /usr/local/bin/rpc_setup.sh

RUN echo "nfs             2049/tcp" >> /etc/services
RUN echo "nfs             111/udp" >> /etc/services
RUN echo "nfs             32764/udp" >> /etc/services
RUN echo "nfs             32765/udp" >> /etc/services
RUN echo "nfs             32766/udp" >> /etc/services
RUN echo "nfs             32767/udp" >> /etc/services
RUN echo "nfs             32768/udp" >> /etc/services
RUN echo "nfs             32769/udp" >> /etc/services
RUN echo "nfs             32764/tcp" >> /etc/services
RUN echo "nfs             32765/tcp" >> /etc/services
RUN echo "nfs             32766/tcp" >> /etc/services
RUN echo "nfs             32767/tcp" >> /etc/services
RUN echo "nfs             32768/tcp" >> /etc/services
RUN echo "nfs             32769/tcp" >> /etc/services
ENV GODEBUG=madvdontneed=1

VOLUME /dgraph-data

EXPOSE 111/udp 2049/tcp 32764 32765 32766 32767 32768 32769
EXPOSE 8080
EXPOSE 9080
RUN mkdir /dgraph
WORKDIR /dgraph
CMD ["dgraph"]
# ENTRYPOINT ["/usr/local/bin/nfs_setup"]
