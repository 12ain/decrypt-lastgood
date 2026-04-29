import io
import struct
import tarfile
from dataclasses import dataclass

from cryptography.hazmat.primitives import hashes, padding
from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC


class LastgoodDecryptError(RuntimeError):
    pass


@dataclass(frozen=True)
class LastgoodContainer:
    header_len: int
    component_1: bytes
    component_2: bytes
    component_3: bytes
    salt: bytes
    encrypted_file_key_hex: str
    encrypted_wrap_key_hex: str
    body: bytes


def parse_container(data: bytes) -> LastgoodContainer:
    if len(data) < 4 + 512:
        raise LastgoodDecryptError("input is too short to be a lastgood container")

    # 厂商容器前 4 字节记录头部长度，头部后面才是按块加密的 tar.gz 正文。
    header_len = struct.unpack_from("<I", data, 0)[0]
    header = data[4 : 4 + header_len]
    body = data[4 + header_len :]
    if len(header) != header_len or len(header) < 514:
        raise LastgoodDecryptError(f"invalid header length: {header_len}")
    if not body or len(body) % 16:
        raise LastgoodDecryptError(f"invalid encrypted body length: {len(body)}")

    try:
        key_text = header[512:].decode("ascii")
        encrypted_file_key_hex, encrypted_wrap_key_hex = key_text.split(", ", 1)
        bytes.fromhex(encrypted_file_key_hex)
        bytes.fromhex(encrypted_wrap_key_hex)
    except Exception as exc:
        raise LastgoodDecryptError("header key material is not in the expected format") from exc

    return LastgoodContainer(
        header_len=header_len,
        component_1=header[0:128],
        component_2=header[128:256],
        component_3=header[256:384],
        salt=header[384:512],
        encrypted_file_key_hex=encrypted_file_key_hex,
        encrypted_wrap_key_hex=encrypted_wrap_key_hex,
        body=body,
    )


def decrypt_lastgood_container(data: bytes) -> bytes:
    container = parse_container(data)
    # 三段 128 字节材料异或后进入 PBKDF2，用于逐层解开 wrap key 和 file key。
    xor_secret = bytes(
        a ^ b ^ c
        for a, b, c in zip(
            container.component_1,
            container.component_2,
            container.component_3,
        )
    )
    wrap2_key = _pbkdf2_sha256_16(xor_secret, container.salt)
    wrap1_key = _aes_cbc_decrypt_iv_prefixed(
        bytes.fromhex(container.encrypted_wrap_key_hex),
        wrap2_key,
    )
    file_key = _aes_cbc_decrypt_iv_prefixed(
        bytes.fromhex(container.encrypted_file_key_hex),
        wrap1_key,
    )
    if len(file_key) != 16:
        raise LastgoodDecryptError(f"unexpected file key length: {len(file_key)}")

    plaintext = bytearray()
    for offset in range(0, len(container.body), 1040):
        # 每个正文块是 16 字节 IV + 最多 1024 字节 AES-CBC 密文。
        chunk = container.body[offset : offset + 1040]
        plaintext.extend(_aes_cbc_decrypt_iv_prefixed(chunk, file_key))
    return bytes(plaintext)


def extract_config_xml(tar_gz_data: bytes) -> bytes:
    try:
        with tarfile.open(fileobj=io.BytesIO(tar_gz_data), mode="r:gz") as archive:
            regular_members = [m for m in archive.getmembers() if m.isfile()]
            if len(regular_members) != 1:
                names = ", ".join(m.name for m in regular_members)
                raise LastgoodDecryptError(f"expected one file in archive, found: {names}")
            extracted = archive.extractfile(regular_members[0])
            if extracted is None:
                raise LastgoodDecryptError("archive member could not be extracted")
            return extracted.read()
    except LastgoodDecryptError:
        raise
    except Exception as exc:
        raise LastgoodDecryptError("decrypted data is not the expected tar.gz archive") from exc


def decrypt_lastgood_xml(data: bytes) -> bytes:
    return extract_config_xml(decrypt_lastgood_container(data))


def _pbkdf2_sha256_16(secret: bytes, salt: bytes) -> bytes:
    return PBKDF2HMAC(
        algorithm=hashes.SHA256(),
        length=16,
        salt=salt,
        iterations=10000,
    ).derive(secret)


def _aes_cbc_decrypt_iv_prefixed(data: bytes, key: bytes) -> bytes:
    if len(key) not in (16, 24, 32):
        raise LastgoodDecryptError(f"invalid AES key length: {len(key)}")
    if len(data) < 32 or len(data) % 16:
        raise LastgoodDecryptError(f"invalid AES blob length: {len(data)}")

    iv = data[:16]
    ciphertext = data[16:]
    decryptor = Cipher(algorithms.AES(key), modes.CBC(iv)).decryptor()
    padded = decryptor.update(ciphertext) + decryptor.finalize()
    try:
        unpadder = padding.PKCS7(128).unpadder()
        return unpadder.update(padded) + unpadder.finalize()
    except ValueError as exc:
        raise LastgoodDecryptError("AES padding check failed") from exc
