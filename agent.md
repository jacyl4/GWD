rm -rf /etc/resolvconf/resolv.conf.d/*
>/etc/resolvconf/resolv.conf.d/original
>/etc/resolvconf/resolv.conf.d/base
>/etc/resolvconf/resolv.conf.d/tail
rm -rf /etc/resolv.conf
rm -rf /run/resolvconf/interface
cat << EOF >/etc/resolvconf/resolv.conf.d/head
nameserver 127.0.0.1
EOF
if [[ -f "/etc/resolvconf/run/resolv.conf" ]]; then
ln -sf /etc/resolvconf/run/resolv.conf /etc/resolv.conf
elif [[ -f "/run/resolvconf/resolv.conf" ]]; then
ln -sf /run/resolvconf/resolv.conf /etc/resolv.conf
fi
sed -i '/dns-nameservers /d' /etc/network/interfaces
resolvconf -u

新建一个模块 internal/configurator/server/resolvconf.go 将上面脚本的操作转化为 go模块。

然后 这个模块需要在 internal/app/server/installer.go 安装序列的 InstallDependencies 后执行。