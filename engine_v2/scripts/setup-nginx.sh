#!/bin/bash
# TuCentroPDF Engine V2 - Nginx Setup Script
# Sets up Nginx reverse proxy with SSL support

set -e

echo "🔧 Setting up Nginx for TuCentroPDF Engine V2..."

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo "❌ Please run as root (sudo)"
    exit 1
fi

# Install Nginx if not present
if ! command -v nginx &> /dev/null; then
    echo "📦 Installing Nginx..."
    apt-get update
    apt-get install -y nginx
fi

# Install Certbot for Let's Encrypt
if ! command -v certbot &> /dev/null; then
    echo "📦 Installing Certbot..."
    apt-get install -y certbot python3-certbot-nginx
fi

# Create necessary directories
mkdir -p /var/www/certbot
mkdir -p /var/www/html
mkdir -p /etc/nginx/sites-available
mkdir -p /etc/nginx/sites-enabled

# Copy Nginx configuration
echo "📝 Copying Nginx configuration..."
cp nginx/nginx.conf /etc/nginx/nginx.conf
cp nginx/sites-available/tucentropdf.conf /etc/nginx/sites-available/tucentropdf.conf

# Test Nginx configuration
echo "🧪 Testing Nginx configuration..."
nginx -t

if [ $? -ne 0 ]; then
    echo "❌ Nginx configuration test failed"
    exit 1
fi

# Get domain from user
read -p "Enter your domain (e.g., tucentropdf.com): " DOMAIN

if [ -z "$DOMAIN" ]; then
    echo "❌ Domain cannot be empty"
    exit 1
fi

# Get email for Let's Encrypt
read -p "Enter your email for Let's Encrypt notifications: " EMAIL

if [ -z "$EMAIL" ]; then
    echo "❌ Email cannot be empty"
    exit 1
fi

# Update domain in Nginx config
sed -i "s/tucentropdf.com/$DOMAIN/g" /etc/nginx/sites-available/tucentropdf.conf

# Enable site
ln -sf /etc/nginx/sites-available/tucentropdf.conf /etc/nginx/sites-enabled/

# Reload Nginx
echo "🔄 Reloading Nginx..."
systemctl reload nginx

# Obtain SSL certificate
echo "🔐 Obtaining SSL certificate from Let's Encrypt..."
echo "This may take a few moments..."

certbot certonly --nginx \
    --non-interactive \
    --agree-tos \
    --email "$EMAIL" \
    -d "$DOMAIN" \
    -d "www.$DOMAIN" \
    -d "api.$DOMAIN"

if [ $? -eq 0 ]; then
    echo "✅ SSL certificate obtained successfully"
else
    echo "⚠️  SSL certificate could not be obtained"
    echo "You can try again later with: certbot certonly --nginx -d $DOMAIN"
fi

# Setup auto-renewal
echo "⏰ Setting up SSL certificate auto-renewal..."
(crontab -l 2>/dev/null; echo "0 3 * * * certbot renew --quiet --post-hook 'systemctl reload nginx'") | crontab -

# Enable and start Nginx
systemctl enable nginx
systemctl restart nginx

echo ""
echo "✅ Nginx setup completed successfully!"
echo ""
echo "📋 Configuration summary:"
echo "  - Domain: $DOMAIN"
echo "  - SSL: Enabled (Let's Encrypt)"
echo "  - Auto-renewal: Enabled (daily at 3 AM)"
echo ""
echo "🔗 Access your API at:"
echo "  - https://$DOMAIN/api/v1/info"
echo "  - https://$DOMAIN/health"
echo ""
echo "⚠️  Make sure your DNS is configured correctly:"
echo "  $DOMAIN        →  Your VPS IP"
echo "  www.$DOMAIN    →  Your VPS IP"
echo "  api.$DOMAIN    →  Your VPS IP"
echo ""
