#!/bin/bash
systemctl stop SystemController
cp bin/ARM/SystemController /usr/bin
systemctl start SystemController
