groupadd --system dgraph
useradd --system -d /var/lib/dgraph -s /bin/false -g dgraph dgraph
mkdir -p /var/log/dgraph
mkdir -p /var/lib/dgraph/{p,w,zw}
chown -R dgraph:dgraph /var/{lib,log}/dgraph
