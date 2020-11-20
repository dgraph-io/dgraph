## Read lines from configuration
lines = File.readlines("./vagrant/hosts")

## Hash of hostname:inet_addr
@hosts = lines.map { |ln| i,h = ln.split(/\s+/); [h,i] }.to_h
## List of systems that will autostart
@starts = lines.select { |ln| ln !~ /nostart/; }.map { |ln| ln.split(/\s+/)[1] }
## Set primary host for `vagrant ssh`
@primary = (lines.select { |ln| ln =~ /primary|default/ }[0] ||="").split[1] || "alpha-1"

## Set Replicas based on # of zeros
@replicas = @hosts.keys.select { |host| host.to_s.match /^zero-\d+/ }.count

## Create hash 0f SMB sync options w/ optional smb_username and smb_password
@smb_sync_opts = { type: "smb", mount_options: %w[mfsymlinks vers=3.0] }
@smb_sync_opts.merge! smb_username: ENV['SMB_USER'] if ENV['SMB_USER']
@smb_sync_opts.merge! smb_password: ENV['SMB_PASSWD'] if ENV['SMB_PASSWD']

## Set Latest Version
uri = URI.parse("https://get.dgraph.io/latest")
response = Net::HTTP.get_response(uri)
latest = JSON.parse(response.body)["tag_name"]
@version = ENV['DGRAPH_VERSION'] || latest
