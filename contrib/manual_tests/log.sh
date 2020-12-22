#!/usr/bin/env bash

function _log_date() {
    date '+%Y-%m-%d %H:%M:%S'
}

function log::debug() {
    printf '%b\n' "\e[32m[DEBUG] $(_log_date) $*\e[0m"
}

function log::info() {
    printf '%b\n' "\e[34m[ INFO] $(_log_date) $*\e[0m"
}

function log::warn() {
    printf '%b\n' "\e[33m[ WARN] $(_log_date) $*\e[0m"
}

function log::error() {
    printf '%b\n' "\e[31m[ERROR] $(_log_date) $*\e[0m"
}
