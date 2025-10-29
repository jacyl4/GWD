cat << "EOF" >/opt/de_GWD/tcpTime
date -s "$(wget -qSO- --max-redirect=0 whatismyip.akamai.com 2>&1 | grep Date: | cut -d' ' -f5-8)Z"
[[ $? -ne "0" ]]&& date -s "$(curl -sI cloudflare.com| grep -i '^date:'|cut -d' ' -f2-)"
hwclock -w
EOF
chmod +x /opt/de_GWD/tcpTime

制作一个 tcp 校时的模块 internal/system/tcptime.go

用于校准系统的时间，算是 ntp 以外的一种手动方式吧。


其中 hwclock -w 这块有没有更通用的方式，因为有些虚拟机是kvm，有些虚拟机可能只是docker