groupadd --system dgraph
useradd --system -d /var/run/dgraph -s /bin/false -g dgraph dgraph
mkdir -p /var/log/dgraph
mkdir -p /var/run/dgraph/{p,w,zw}
chown -R dgraph:dgraph /var/{run,log}/dgraph
