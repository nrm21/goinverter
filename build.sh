#!/bin/bash

PREVDIR=$(pwd)
cd /opt/goinverter

go build -o ./bin/goinverter.new ./src
mv /opt/goinverter/bin/goinverter /opt/goinverter/bin/goinverter.bak
mv /opt/goinverter/bin/goinverter.new /opt/goinverter/bin/goinverter

cd $PREVDIR
