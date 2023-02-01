FROM ubuntu:20.04
LABEL maintainer="Dgraph Labs <contact@dgraph.io>"

ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update -qq && apt-get install -y nfs-kernel-server runit inotify-tools -qq
RUN mkdir -p /dgraph-data
RUN apt-get install -y --no-install-recommends ca-certificates
RUN apt-get install -y --no-install-recommends curl
RUN apt-get install -y --no-install-recommends htop
RUN apt-get install -y --no-install-recommends iputils-ping
RUN apt-get install -y --no-install-recommends jq
RUN apt-get install -y --no-install-recommends less
RUN apt-get install -y --no-install-recommends sysstat

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
