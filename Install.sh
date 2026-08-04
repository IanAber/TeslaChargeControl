#!/bin/bash
if [ "$USER" != "root" ]
then
    echo "Please run this as root or with sudo"
    exit 2
fi

systemctl stop SystemController
cp bin/ARM/SystemController /usr/bin
cp -r web/* /var/www/html
systemctl start SystemController
