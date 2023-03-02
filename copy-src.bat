@rem ensure id_rsa pub/priv files in C:\Users\Nate\.ssh first

ssh nate@pidawg2 "rm -rf /opt/goinverter/src/*"
scp -r C:/Users/Nate/go/src/_nate/goinverter/src nate@pidawg2:/opt/goinverter/
@rem ssh pidawg2 "chmod 755 /opt/goinverter/.git && chmod 755 /opt/goinverter/bin && chmod 755 /opt/goinverter/src"
