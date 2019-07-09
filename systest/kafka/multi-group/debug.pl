#!/usr/bin/env perl
use strict;


if (@ARGV < 1) {
    die "Usage: debug.pl dest_alpha"
}
my $dstAlpha = $ARGV[0];
print "$dstAlpha\n";

my @consumer_up = `docker logs $dstAlpha 2>&1|grep -i "consumer up and running"`;
print @consumer_up;

open(LOG200, "docker logs $dstAlpha 2>&1|grep -i Kafka|grep 'source partition'|") or die "cannot read partition for $dstAlpha";
my $partition = -1;
while(<LOG200>) {
    if ($_ =~ /partition (\d)$/) {
        $partition = $1;
    }
}
close(LOG200);

if ($partition < 0) {
    die "cannot find the partition for $dstAlpha";
}
print "got partition for $dstAlpha: $partition\n";

my $source_alpha = "";
my @source_alphas = ("alpha100", "alpha101", "alpha102");
for my $sa (@source_alphas) {
    open(SALOG, "docker logs $sa 2>&1|grep -i 'target partition'|");
    while(<SALOG>) {
        if ($_=~/target partition \Q$partition/) {
            $source_alpha = $sa;
        }
    }
    close(SALOG);
}

if ($source_alpha eq "") {
die "cannot find the source alpha for partition $partition";
}

print "source alpha: $source_alpha\n";
