# 前端镜像（nginx 静态托管 + 反向代理到 gateway）
#
# Build context = zeus repo 根：
#   docker build -t zeus-frontend -f examples/20-full-demo/docker/frontend.Dockerfile ..

FROM nginx:alpine

# 拷贝前端静态资源
COPY examples/20-full-demo/frontend/ /usr/share/nginx/html/

# nginx 配置：静态托管 + /api、/login 反向代理到 gateway
RUN printf 'server {\n\
    listen 80;\n\
    server_name _;\n\
    root /usr/share/nginx/html;\n\
    index index.html;\n\
\n\
    location / { try_files $uri $uri/ /index.html; }\n\
\n\
    location /api/ {\n\
        proxy_pass http://gateway:8080;\n\
        proxy_set_header Host $host;\n\
    }\n\
    location /login {\n\
        proxy_pass http://gateway:8080;\n\
        proxy_set_header Host $host;\n\
        proxy_set_header X-Zeus-Cluster $http_x_zeus_cluster;\n\
    }\n\
    location /health { return 200 "ok"; }\n\
}\n' > /etc/nginx/conf.d/default.conf

EXPOSE 80
