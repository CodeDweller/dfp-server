# dfp-server
Distributed fuzzing platform server

Distributed fuzzing platform based on server-client architecture, primarily designed for higly distributed fuzzing using [WinAFL](https://github.com/googleprojectzero/winafl). With slight modifications, the system can be used for any command&control type of distributed tasks.
#
Clients connect to the server, which in turn stores all relevant client info in Redis. Non-real time communication uses a standard REST API, while more immediate client control uses websockets. 
