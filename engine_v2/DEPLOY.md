# TuCentroPDF Engine V2 - Production Deployment Guide

## 🚀 VPS Production Deployment

This guide provides step-by-step instructions for deploying TuCentroPDF Engine V2 to a production VPS environment.

## 📋 Prerequisites

### System Requirements
- **CPU**: Minimum 2 cores, Recommended 4+ cores
- **RAM**: Minimum 4GB, Recommended 8GB+ 
- **Storage**: Minimum 20GB free space
- **OS**: Ubuntu 22.04 LTS, Debian 12, CentOS 8+, or RHEL 8+

### Required Software
- Docker Engine 24.0+
- Docker Compose 2.0+
- Git 2.30+
- OpenSSL (for certificate generation)

### Required Accounts/Keys
- OpenAI API Key (for AI OCR functionality)
- Domain name (recommended for production)

## 🔧 Server Setup

### 1. Update System
```bash
# Ubuntu/Debian
sudo apt update && sudo apt upgrade -y

# CentOS/RHEL
sudo dnf update -y
```

### 2. Install Docker
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# CentOS/RHEL
sudo dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
sudo dnf install docker-ce docker-ce-cli containerd.io docker-compose-plugin -y
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
```

### 3. Install Docker Compose (if not included)
```bash
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
```

### 4. Configure Firewall
```bash
# Ubuntu/Debian (UFW)
sudo ufw allow ssh
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 8080/tcp  # Engine port (or use nginx proxy)
sudo ufw --force enable

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-service=ssh
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload
```

## 📦 Deployment Steps

### 1. Clone Repository
```bash
git clone https://github.com/yourusername/tucentropdf-engine.git
cd tucentropdf-engine/engine_v2
```

### 2. Configure Environment
```bash
# Copy production environment template
cp .env.production .env

# Edit configuration
nano .env
```

**Required Configuration:**
```env
ENGINE_SECRET=your-super-secure-secret-minimum-32-characters-long
OPENAI_API_KEY=your-openai-api-key-here
ENVIRONMENT=production
LOG_LEVEL=info
```

### 3. Create Required Directories
```bash
mkdir -p temp logs monitoring
chmod 755 temp logs
```

### 4. Build and Deploy

#### Basic Deployment (Engine + Redis)
```bash
docker-compose up -d
```

#### Full Deployment with Monitoring
```bash
docker-compose --profile monitoring up -d
```

#### Deployment with Gotenberg (Office Fallback)
```bash
docker-compose --profile gotenberg up -d
```

#### Complete Deployment (All Services)
```bash
docker-compose --profile monitoring --profile gotenberg up -d
```

### 5. Verify Deployment
```bash
# Check service status
docker-compose ps

# Check logs
docker-compose logs tucentropdf-engine

# Test health endpoint
curl http://localhost:8080/health
```

## 🔒 Production Security

### 1. Generate Secure Secrets
```bash
# Generate ENGINE_SECRET
openssl rand -base64 48

# Store in .env file
echo "ENGINE_SECRET=$(openssl rand -base64 48)" >> .env
```

### 2. Setup SSL/TLS with Nginx

Create nginx configuration:
```bash
sudo mkdir -p /etc/nginx/sites-available
sudo tee /etc/nginx/sites-available/tucentropdf > /dev/null <<EOF
server {
    listen 80;
    server_name yourdomain.com www.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com www.yourdomain.com;

    ssl_certificate /etc/ssl/certs/yourdomain.com.crt;
    ssl_certificate_key /etc/ssl/private/yourdomain.com.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;

    client_max_body_size 100M;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

sudo ln -s /etc/nginx/sites-available/tucentropdf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

### 3. Setup Let's Encrypt SSL
```bash
sudo apt install certbot python3-certbot-nginx -y
sudo certbot --nginx -d yourdomain.com -d www.yourdomain.com
```

## 📊 Monitoring Setup

### 1. Access Monitoring Services
- **Grafana**: http://your-server:3001 (admin/your-grafana-password)
- **Prometheus**: http://your-server:9090

### 2. Configure Grafana Dashboards
1. Login to Grafana
2. Add Prometheus data source: http://prometheus:9090
3. Import dashboard ID: 1860 (Node Exporter Full)
4. Create custom dashboard for engine metrics

## 🔄 Maintenance

### 1. Log Management
```bash
# View logs
docker-compose logs -f tucentropdf-engine

# Log rotation is configured automatically
# Logs are limited to 10MB per file, 3 files max
```

### 2. Updates
```bash
# Pull latest changes
git pull origin main

# Rebuild and restart
docker-compose build --no-cache
docker-compose down
docker-compose up -d
```

### 3. Backup Strategy
```bash
# Backup script
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p backups

# Backup Redis data
docker-compose exec redis redis-cli BGSAVE
docker cp $(docker-compose ps -q redis):/data/dump.rdb backups/redis_$DATE.rdb

# Backup logs
tar -czf backups/logs_$DATE.tar.gz logs/

# Backup configuration
cp .env backups/env_$DATE.backup
```

### 4. Performance Tuning
```bash
# Monitor resource usage
docker stats

# Adjust resource limits in docker-compose.yml
# Monitor with Grafana dashboards
```

## 🚨 Troubleshooting

### Common Issues

1. **Out of Memory**
   ```bash
   # Check memory usage
   free -h
   docker stats
   
   # Increase swap if needed
   sudo fallocate -l 2G /swapfile
   sudo chmod 600 /swapfile
   sudo mkswap /swapfile
   sudo swapon /swapfile
   ```

2. **Port Already in Use**
   ```bash
   # Check what's using the port
   sudo netstat -tlnp | grep :8080
   
   # Change port in .env
   echo "ENGINE_PORT=8081" >> .env
   ```

3. **SSL Certificate Issues**
   ```bash
   # Renew certificates
   sudo certbot renew
   
   # Test certificate renewal
   sudo certbot renew --dry-run
   ```

### Health Checks
```bash
# Engine health
curl -f http://localhost:8080/health

# Redis health
docker-compose exec redis redis-cli ping

# Check all services
docker-compose ps
```

## 📞 Support

For production support:
- Check logs: `docker-compose logs`
- Monitor resources: Access Grafana dashboard
- Review configuration: Validate `.env` file
- Test endpoints: Use provided API documentation

## 🔧 Advanced Configuration

### Environment Variables Reference
See `.env.production` template for complete configuration options.

### Resource Scaling
Adjust resource limits in `docker-compose.yml` based on your VPS specifications and load requirements.

### Load Balancing
For high-traffic deployments, consider setting up multiple engine instances behind a load balancer.