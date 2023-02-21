#!/bin/bash

rsync -crv nate@workdawg1:/home/nate/cephfs/linux/programming/goinverter /opt
go build -o ./bin/goinverter ./src
