#!\bin\bash

# 指定证书的有效期
DAYS=36500

# 生成私钥
openssl genrsa -out key.pem 2048

# 生成根证书
openssl req -x509 -new -nodes -key key.pem -sha256 -days ${DAYS} -out cert.pem -subj "/\C=CN/\ST=GD/\L=SZ/\O=PYJ/\OU=IT/\CN=PYJ.com"

echo "生成的私钥文件：key.pem"
echo "生成的自签根证书：cert.pem"
cp cert.pem cert.crt
