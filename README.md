# anlGoLB

AnlGoLB: 负载均衡程序

1、基于Golang语言开发

2、支持https、http、websocket协议

3、仅仅支持反向代理，正向代理等后期根据需要在考虑增加

4、当前版本儿未做策略限制，负载算法使用rand


安装运行：
go get github.com/Ysach/anlGoLB

进入到$GOAPATH的src目录，配置 config.json文件

target： 您的proxy backend server

port：您的proxy server 端口

verbose： 是否允许 backend server 不同的协议 https/http true：只能使用backend server 只能使用 https 或 http协议， false 可以使用 http + https 协议

SSL：proxy server 是否使用 https协议  true： --> https， false： ---> http
