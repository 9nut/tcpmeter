**tcpmeter** - a tool for measuring TCP upload and download speeds and RTT latency.

### Build

```shell
go build
```

#### Run

* start the server on the remote machine:

  `tcpmeter -s -r $(hostname):8001`

* start the client on the local machine:

  `tcpmeter -c`

  then navigate to `http://localhost:8080` using an HTML5 browser to interact with the client.

## Documentation

 `godoc`

## License
 MIT license (see LICENSE file).

## Contact
 `skip.tavakkolian@gmail.com`
	
## Screenshot
![alt text](https://github.com/9nut/tcpmeter/raw/master/tcpmeter.png "tcpmeter client web UI")

