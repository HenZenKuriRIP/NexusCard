# NexusCard


## 一键安装

服务器需为 Linux（amd64 / arm64），域名 DNS **A 记录** 指向本机。

```bash
curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh \
  | sudo bash -s -- pay.example.com
```

固定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh \
  | sudo VERSION=v1.0.0 bash -s -- pay.example.com
```

安装完成后查看：`/opt/giftcard-platform/README-DEPLOY.txt`

---

## 一键卸载

```bash
curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/NexusCard/main/deploy/install.sh \
  | sudo bash -s -- --uninstall
```

卸载仅移除服务与 Nginx 站点配置；**数据库、配置与域名证书默认保留**，便于重装复用。
