#!/usr/bin/env ruby

# Purpose:
#  This builds a list of mappings of AMI images in either YAML or JSON
#  for use with CFN (CloudFormation) scripts.
# Background:
#  In AWS, each region has unique AMI id for the desired images, so
#  you need to build a list of target AMI IDs for use with your scripts.
# Requirements:
#  aws cli tools with profile (~/.aws/) configured
#
require 'yaml'
require 'json'

# main
#  main
def main
  # get arguments
  (mode, owner, filter) = parse_arguments

  # print results in JSON or YAML
  print_mappings(mode, owner, filter)
end

# parse_arguments
#   process command line arguments
def parse_arguments
  # get command line arguments
  (mode, owner, filter) = ARGV[0, 2]

  # set to defaults if not set
  mode = 'json' if mode.nil? || mode.empty?
  owner = 'canonical' if owner.nil? || owner.empty?
  # default filter for Ubuntu 18.04 Bionic
  if filter.nil? || filter.empty?
    filter = 'ubuntu/images/hvm-ssd/ubuntu-bionic-*amd64-server*'
  end

  [mode, owner, filter]
end

# print_mappings
#   output final rendered result
def print_mappings(mode, owner, filter)
  mappings = build_ami_mappings(owner, filter)

  if mode =~ /json/
    puts JSON.pretty_generate(mappings)
  elsif mode =~ /yaml|yml/
    puts mappings.to_yaml
  end
end

# list_regions
#  return a list of regions that are accessible given your profile
def list_regions
  `aws ec2 describe-regions --query "Regions[].{Name:RegionName}" --output text`
end

# get_latest_image
#  returns the most recent image given the owner and filter
def get_latest_image(owner, filter)
  owners = { canonical: '099720109477' }

  images = `aws ec2 describe-images \
    --owners #{owners[owner]} \
    --filters "Name=name,Values=#{filter}" \
    --query 'sort_by(Images, &CreationDate)[].Name' \
    --output text`

  # return latest
  images.split[-1]
end

# get_region_ami_id
#  returns ami for a given region given an example image name for the filter
def get_region_ami_id(owner, filter, region)
  owners = { canonical: '099720109477' }

  ami_id = `aws ec2 describe-images \
    --region #{region} \
    --owners #{owners[owner]} \
    --filters "Name=name,Values=#{filter}" \
    --query Images[].ImageId \
    --output text`.chomp

  ami_id
end

# build_ami_mappings
#   returns a hash of AMI mappings
def build_ami_mappings(owner, filter)
  ami_mappings = {}
  image_name = get_latest_image(owner, filter)
  list_regions.split.each do |region|
    ami_id = get_region_ami_id(owner, image_name, region)
    ami_mappings.merge!({ region => { '64' => ami_id } })
  end

  # return final structure
  { 'Mappings' => { 'AWSRegionArch2AMI' => ami_mappings } }
end

main
