# decrypt-lastgood

该工具可用于下载和解密移动光猫的lastgood.xml文件，适用于型号UNG953H1-S，其它型号未测试


## 前置条件
- 需要先用普通用户登录光猫，然后通过`http://192.168.1.1/webcmcc/telnet_page.html` 页面打开telnet

## 安装使用
    1.下载源码到本地 
    2.在项目根目录下执行: uv sync


## 下载加密文件

```bash
uv run dlg download 192.168.1.1 -u '<user>' -p '<password>' -o lastgood.xml.enc
```

## 解密本地文件

```bash
uv run dlg decrypt lastgood.xml.enc -o lastgood.xml
```

## 备注
- 解密后的文件中 aucTeleAccountName对应超级用户，aucTeleAccountPassword对应超级密码
- 本项目仅限个人研究学习，请勿用于其他用途
- 使用完后请注意关闭光猫上的telnet