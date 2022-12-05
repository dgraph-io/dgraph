FROM ubuntu:20.04
ENV DEBIAN_FRONTEND noninteractive
RUN apt-get update -qq && apt-get install -y nfs-kernel-server runit inotify-tools -qq
RUN mkdir -p /dgraph-data

COPY setups/nfs-common /etc/default/nfs-common
COPY setups/nfs-kernel-server /etc/default/nfs-kernel-server
COPY setups/quota /etc/default/quota
COPY setups/nfs-static-ports.conf /etc/sysctl.d/nfs-static-ports.conf
COPY setups/nfs-static-ports.conf /etc/sysctl.conf
COPY setups/nfs /etc/sysconfig/nfs

RUN mkdir -p /etc/sv/nfs
ADD nfs.init /etc/sv/nfs/run
ADD nfs.stop /etc/sv/nfs/finish

ADD nfs_setup.sh /usr/local/bin/nfs_setup

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

VOLUME /dgraph-data

EXPOSE 111/udp 2049/tcp 32764 32765 32766 32767 32768 32769

ENTRYPOINT ["/usr/local/bin/nfs_setup"]
