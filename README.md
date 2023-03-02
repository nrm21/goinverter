# goinverter

This is a simple go program that does several things when run:
* It queries the USB bus of an Axpert-type Solar Inverter (Voltronic, Axpert, MPPSolar, EASunpower, etc...) and feeds the data recieved to a web listener
* It runs a web listener/server for clients to either query that recieved data in JSON format or simply send commands to the inverter directly
* The backend polling of the USB and the web listener are completely decoupled so you can hit the web listener as fast as you want and it will reserve the same data until the poller refreshes it with new info from the inverter

The program stays resident when run and does not quit unless sent SIGTERM.  The queries to USB are done at a specified interval (default of 12 seconds but can be set at the CLI by parameter).  The program was designed to be lightweight and run on a Rasperry Pi B+, and also designed to be a standalone executable with no major outside dependancies.

### Instructions
To run at the CLI, assuming it's in it's own directory within the opt directory...
```
/opt/goinverter/goinverter [-d] [-i 15] [-p 8088]

Optional parameters
  -d               -- sets the debug flag (very verbose output)
  -i <interval>    -- sets the interval to poll the USB bus of the solar inverter
  -p <port number> -- the port to run the web listener on
```

### Motivation
My motivation for writing this program was to have a lightweight program that would run on my RasPi B+ (since that's what I have connected directly to my solar inverter).  I didn't feel like wasting a full windows/linux laptop to sit over in the corner just to be connected to the box and read values from it.  Rather I wanted to be able to get that data from across the network.  
A few years back I found manio's (Mariusz Białończyk) skymax C++ program that did mostly what I need and, after making some modifications to it, I used that for a few years for my needs.  This mostly consisted of gathering the data, shoving it into a time-series DB and using grafana to view it.  After some time I decided to write it in a language that I prefer and make it behave more like what I am after, a RESTful web listener that can accept connections from any number of clients.  The idea was that I could possibly build a windows GUI to connect to this across a network in the future if there is time.  And/or by putting it on github, perhaps someone else will like it and decide to add a frontent GUI program.
