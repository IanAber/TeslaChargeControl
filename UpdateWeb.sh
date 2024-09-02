#!/bin/bash
if [ "$USER" != "root" ]
then
    echo "Please run this as root or with sudo"
    exit 2
fi

cp -r web/* /var/www/html
