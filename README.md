# go-keepAlive


go-keepAlive is HA tool witch checks services health on multiple services nodes and updates zookeeper node defined for specific service with healthy services nodes.  

### Quick start

- create app configuration file, config.yml, like bellow:  
```
---
services:
        - name: test_service
          nodes:
                - ip: 172.217.16.99
                  port: 80
                - ip: 172.217.16.110
                  port: 80
```

- start app in master mode:
`go_keepAlive -mode master`  
App connects to zookeeper(s) defined with `-zk` flag or localhost by default

- start app in agent node on three servers:
`go_keepalive`  
App in agent node does not need configuration, it reads it from zookeeper.
It also requires `-zk` flag if zookeeper is not on localhost.

- read contents of service node, representing healthy nodes:  
```
[zk: localhost:2181(CONNECTED) 17] get /go-keepAlive/services/test_service
172.217.16.99 172.217.16.110
cZxid = 0xf3d6
ctime = Tue Nov 07 15:31:52 UTC 2017
mZxid = 0xf3eb
mtime = Thu Nov 09 11:12:00 UTC 2017
pZxid = 0xf3d9
cversion = 2
dataVersion = 2
aclVersion = 0
ephemeralOwner = 0x0
dataLength = 28
numChildren = 2
```

### Notes
The project is still in early alpha release, lacking a lot of features, and with potential bugs.
