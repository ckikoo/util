# 声明: 此库仅供学习参考，禁止用于商业用途，否则后果自负。
> 聚合多个网站下载（虽然没啥用）， 支持镜像包缓存， 加快下载。
> 镜像包被限制100M， 因为弄到阿里云盘，web下载限制100M 

# 说明
> 端口位 23000
> 
> 使用请自行修正 /token的url
> 
> 添加使用addurl 直接在main里弄

# 系统设置镜像加速
修改文件 /etc/docker/daemon.json（如果不存在则创建）
```
sudo mkdir -p /etc/docker
 sudo tee /etc/docker/daemon.json <<-'EOF'
 {
  "registry-mirrors": ["https://docker.fxxk.dedyn.io"]  # 请替换为您自己的Worker自定义域名
 }
 EOF
 sudo systemctl daemon-reload
 sudo systemctl restart docker
```
# 官方镜像路径前面加域名
```
docker pull ip:prot(或者域名)/stilleshan/frpc:latest
```
```
docker pull ip:prot(或者域名)/library/nginx:stable-alpine3.19-perl
```

# 参考项目
[CF-Workers-docker.io](https://github.com/ckikoo/CF-Workers-docker.io)
