#!/bin/bash

if [ `basename "$0"` == "_vsdbg.sh" ]; then
	curl -sSL https://aka.ms/getvsdbgsh | /bin/sh /dev/stdin -v latest -l ~/vsdbg
	apt update && apt install -y procps openssh-server
	sed -i.bak 's/^#PermitRootLogin .*/PermitRootLogin yes/g' /etc/ssh/sshd_config
	echo "root:toor" | chpasswd
	service ssh start
	exit
fi

if [ "$1" == "" ]; then
	echo "please specify container id"
	exit
fi
echo $1
docker cp vsdbg.sh $1:/_vsdbg.sh
docker exec $1 bash /_vsdbg.sh
containerip=$(docker inspect -f '{{range $key, $value := .NetworkSettings.Networks}}{{if ne $key "ingress"}}{{$value.IPAddress}}{{end}}{{end}}' $1)
networkid=$(docker inspect -f '{{range $key, $value := .NetworkSettings.Networks}}{{if ne $key "ingress"}}{{$value.NetworkID}}{{end}}{{end}}' $1)
echo "starting tunnel to $containerip on network $networkid on port $2"
docker run --rm --network $networkid -p $2:22  alpine/socat tcp-listen:22,fork,reuseaddr tcp-connect:$containerip:22
