# `lastgood.xml` 容器格式

九联 `UNG953H1-S`（XGPON ONU）把 `/config/work/lastgood.xml` 存成一种加密容器。
下面是布局。格式是黑盒逆向出来的，只覆盖实测过的 V1.0.02，实现在
[`internal/container`](../internal/container)。

## 头部

文件开头 4 字节是小端 uint32，记作 `L`，表示头部长度。头部从第 4 字节开始，共 `L` 字节：

| 偏移（相对 header） | 长度 | 字段 | 说明 |
| --- | --- | --- | --- |
| 0 | 128 | `cmp1` | 密钥材料分片 1 |
| 128 | 128 | `cmp2` | 密钥材料分片 2 |
| 256 | 128 | `cmp3` | 密钥材料分片 3 |
| 384 | 128 | `salt` | PBKDF2 盐 |
| 512 | `L - 512` | `key material` | ASCII，见下 |

约束：`L ≥ 514`，`4 + L ≤ 文件长度`。

## 密钥材料

`header[512:]` 是 ASCII 文本，两个十六进制串用 `", "` 分隔：

```
<encrypted_file_key>, <encrypted_wrap_key>
```

两者都是 IV 前缀的 AES-CBC 密文：前 16 字节是 IV，后面是密文，密文长度是 16 的倍数，
整体不短于 32 字节。

## 正文

`body` 从 `4 + L` 开始，长度必须是 16 的倍数且非空。按固定 1040 字节切块，最后一块可以
短一些，同样要满足 16 字节对齐。

## 解密

```
L      = LE_uint32(blob[0:4])
header = blob[4 : 4+L]
body   = blob[4+L : ]

secret = cmp1[i] ^ cmp2[i] ^ cmp3[i]        for i in 0..127
wrap2  = PBKDF2-HMAC-SHA256(secret, salt, 10000, 16)
wrap1  = AES_CBC_DECRYPT(encrypted_wrap_key, wrap2)
file_k = AES_CBC_DECRYPT(encrypted_file_key, wrap1)

plain = b""
for chunk in split(body, 1040):
    plain += AES_CBC_DECRYPT(chunk, file_k)

return gunzip_untar_single(plain)
```

`AES_CBC_DECRYPT(data, key)` 就是取前 16 字节当 IV，解剩下的密文，再去掉 PKCS#7 填充。

两层的顺序不能反：先用 `encrypted_wrap_key` 解出 `wrap1_key`（16 字节），再用它去解
`encrypted_file_key` 得到 `file_key`（16 字节）。每层解出来的长度都得是 16，否则直接判
失败。

## 分块细节

每块 1040 字节 = 16 字节 IV + 至多 1024 字节密文。因为 PKCS#7 总要追加 1 到 16 字节填充，
明文上限是 1008：

| 明文长度 | 填充 | 密文 | 块总长 |
| --- | --- | --- | --- |
| 1008 | 16 | 1024 | 1040 |
| 1023 | 1 | 1024 | 1040 |
| 1024 | 16 | 1040 | 1056，超出块长 |

## 载荷

明文是 gzip 压缩的 tar 归档，里面只有一个普通文件 `lastgood.xml`。多于一个当格式异常
处理，不要静默取第一个。

| 字段 | 含义 |
| --- | --- |
| `aucTeleAccountName` | 超级管理员账号（通常是 CMCCAdmin） |
| `aucTeleAccountPassword` | 超级管理员密码 |
| `aucTeleAccountPasswordHash` | 密码哈希，本工具不处理 |

字段可能被 CDATA 包裹或做过 XML 实体转义，提取时要还原。

## 失败模式

| 现象 | 含义 |
| --- | --- |
| 文件过短 | 不是该容器，或传输被截断 |
| header length 越界 | 头部损坏，或字节序误判成大端 |
| body length 不是 16 的倍数 | 文件不完整，telnet 传输常见 |
| 密钥材料分隔符缺失 | 头部布局不符合预期，可能是别的固件版本 |
| PKCS#7 填充校验失败 | 密钥错了，或块边界错位 |
| 解出来不是 tar.gz | 上面某一步错了 |

填充校验失败反而好排查，它在密钥链每一层都会触发，能直接看出是 `wrap1_key` 错了还是
`file_key` 错了。

## 未确认

- V1.0.03 及以后是否还是这个格式
- 同系列 `UNG953H-S` 是否一致
- 三段 component 和 `salt` 有没有跟设备 SN / MAC 绑定

遇到上面任何一种情况，提 issue 时附上 `wc -c` 的字节数和卡在哪一步就够了。完整的密钥
材料和解密后的 XML 不要贴出来。
