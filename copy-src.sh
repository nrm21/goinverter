#!/bin/bash

# ensure id_rsa pub/priv files in C:\Users\Nate\.ssh first
ssh pidawg2 "rm -rf /opt/goinverter/src/*"
scp -r C:/Users/Nate/go/src/_nate/goinverter/src pidawg2:/opt/goinverter/
#ssh pidawg2 "chmod 755 /opt/goinverter/.git && chmod 755 /opt/goinverter/bin && chmod 755 /opt/goinverter/src"
