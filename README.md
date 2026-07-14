# NexusCard


## 一键安装

服务器需为 Linux（amd64 / arm64），**root 登录**（不要加 `sudo`），域名 DNS **A 记录** 指向本机。

先下载脚本，再执行（**不要**使用 `curl | bash` 管道）：

```bash
curl -fsSL -o /tmp/nexuscard-install.sh \
  https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh
bash /tmp/nexuscard-install.sh pay.example.com
```

固定版本：

```bash
curl -fsSL -o /tmp/nexuscard-install.sh \
  https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh
VERSION=v1.1.1 bash /tmp/nexuscard-install.sh pay.example.com
```

安装完成后查看：`/opt/giftcard-platform/README-DEPLOY.txt`

---

## 一键卸载

同样先下载再执行（root，不要 `sudo`，不要管道）：

```bash
curl -fsSL -o /tmp/nexuscard-install.sh \
  https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh
bash /tmp/nexuscard-install.sh --uninstall
```

若本机已有安装脚本可直接：

```bash
bash /tmp/nexuscard-install.sh --uninstall
```

卸载仅移除服务与 Nginx 站点配置；**数据库、配置与域名证书默认保留**，便于重装复用。
