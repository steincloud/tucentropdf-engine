# TuCentroPDF Engine V2 - Secrets Management Documentation

## 🔐 Overview

This document describes how to manage secrets in TuCentroPDF Engine V2 production environment.

## Secrets Types

### 1. Critical Secrets (MUST be unique per deployment)

- **ENGINE_SECRET** (64 chars): Core engine encryption key
- **JWT_SECRET** (RSA 4096): Private key for JWT token signing
- **JWT_PUBLIC_KEY** (RSA 4096): Public key for JWT verification
- **REDIS_PASSWORD** (32 chars): Redis database password
- **POSTGRES_PASSWORD** (32 chars): PostgreSQL database password
- **ENCRYPTION_KEY** (64 hex): AES-256 key for backup encryption
- **SIGNING_KEY** (128 hex): HMAC-SHA512 key for legal audit signing

### 2. API Secrets (External services)

- **OPENAI_API_KEY**: OpenAI API key for AI OCR
- **SMTP credentials**: Email server authentication
- **Telegram tokens**: Bot notifications

## 🔧 Initial Setup

### Option 1: Automated Generation (Recommended)

```bash
cd engine_v2
chmod +x scripts/generate-secrets.sh
./scripts/generate-secrets.sh
```

This will:
1. Generate cryptographically secure secrets
2. Create `.env.production` with all secrets
3. Backup existing `.env.production` if present

### Option 2: Manual Generation

```bash
# ENGINE_SECRET (64 chars)
openssl rand -base64 48 | tr -d "=+/" | cut -c1-64

# JWT Keys (RSA 4096)
openssl genrsa -out jwt_private.pem 4096
openssl rsa -in jwt_private.pem -pubout -out jwt_public.pem
JWT_SECRET=$(cat jwt_private.pem | base64 -w 0)
JWT_PUBLIC_KEY=$(cat jwt_public.pem | base64 -w 0)

# REDIS_PASSWORD (32 chars)
openssl rand -base64 32 | tr -d "=+/" | cut -c1-32

# ENCRYPTION_KEY (64 hex)
openssl rand -hex 32

# SIGNING_KEY (128 hex)
openssl rand -hex 64
```

## 🔄 Secrets Rotation

Secrets should be rotated periodically for security:

- **Critical secrets**: Every 90 days
- **API keys**: When compromised or annually
- **Passwords**: Every 90 days

### Rotation Process

```bash
# 1. Generate new secrets
./scripts/rotate-secrets.sh

# 2. Update production environment
# Edit .env.production with new values

# 3. Restart services
docker-compose -f docker-compose.prod.yml restart

# 4. Verify
./scripts/health-check.sh
```

### Rotation Script

```bash
#!/bin/bash
# scripts/rotate-secrets.sh

# Generate new ENGINE_SECRET
NEW_SECRET=$(openssl rand -base64 48 | tr -d "=+/" | cut -c1-64)
sed -i.bak "s/ENGINE_SECRET=.*/ENGINE_SECRET=$NEW_SECRET/" .env.production

# Generate new JWT keys
openssl genrsa -out /tmp/jwt_new.pem 4096 2>/dev/null
JWT_SECRET=$(cat /tmp/jwt_new.pem | base64 -w 0)
sed -i "s|JWT_SECRET=.*|JWT_SECRET=$JWT_SECRET|" .env.production
rm -f /tmp/jwt_new.pem

# Generate new REDIS_PASSWORD
NEW_REDIS=$(openssl rand -base64 32 | tr -d "=+/" | cut -c1-32)
sed -i "s/REDIS_PASSWORD=.*/REDIS_PASSWORD=$NEW_REDIS/" .env.production

echo "✅ Secrets rotated. Backup saved as .env.production.bak"
echo "⚠️  Restart services: docker-compose restart"
```

## 🔒 Security Best Practices

### 1. File Permissions

```bash
# Restrict access to .env.production
chmod 600 .env.production
chown root:root .env.production  # If running as root

# Restrict scripts directory
chmod 700 scripts/
```

### 2. Git Protection

Add to `.gitignore`:
```
.env.production
.env.production.*
*.pem
*.key
backups/
```

### 3. Secure Storage

**DO NOT**:
- ❌ Commit secrets to Git
- ❌ Store in plaintext on shared drives
- ❌ Send via email or Slack
- ❌ Include in logs or error messages

**DO**:
- ✅ Use `.env.production` only on production server
- ✅ Store backups encrypted
- ✅ Use secrets management tools (Vault, AWS Secrets Manager)
- ✅ Rotate regularly
- ✅ Audit access logs

### 4. Production Secrets Manager (Advanced)

For enterprise deployments, integrate with:

**HashiCorp Vault**:
```bash
# Store secret in Vault
vault kv put secret/tucentropdf/engine-secret value="$ENGINE_SECRET"

# Retrieve in startup script
ENGINE_SECRET=$(vault kv get -field=value secret/tucentropdf/engine-secret)
```

**AWS Secrets Manager**:
```bash
# Store secret
aws secretsmanager create-secret \
    --name tucentropdf/engine-secret \
    --secret-string "$ENGINE_SECRET"

# Retrieve in application
aws secretsmanager get-secret-value \
    --secret-id tucentropdf/engine-secret \
    --query SecretString --output text
```

## 🚨 Compromise Response

If secrets are compromised:

### Immediate Actions

1. **Rotate ALL secrets immediately**:
```bash
./scripts/generate-secrets.sh
mv .env.production .env.production.compromised
cp .env.production.backup .env.production
```

2. **Revoke exposed API keys**:
- OpenAI Dashboard → Revoke API key
- SMTP provider → Change password

3. **Restart all services**:
```bash
docker-compose -f docker-compose.prod.yml down
docker-compose -f docker-compose.prod.yml up -d
```

4. **Audit logs**:
```bash
# Check for suspicious activity
docker logs tucentropdf-engine | grep -i "error\|fail\|unauthorized"
```

5. **Notify users** (if JWT compromised):
- Force re-login for all users
- Invalidate all sessions in Redis

### Post-Incident

1. Review access logs
2. Update incident response documentation
3. Implement additional security measures
4. Schedule security audit

## 📋 Secrets Validation

Before deployment, validate secrets:

```bash
# Run validation script
./scripts/pre-deploy-check.sh

# Manual validation
if [ ${#ENGINE_SECRET} -lt 32 ]; then
    echo "❌ ENGINE_SECRET too short"
fi

if [[ $OPENAI_API_KEY == sk-* ]]; then
    echo "✅ OpenAI key format valid"
fi
```

## 📊 Secrets Checklist

Before going to production:

- [ ] All secrets generated with `generate-secrets.sh`
- [ ] OPENAI_API_KEY configured
- [ ] CORS_ORIGINS updated with production domains
- [ ] SMTP credentials configured (if using email alerts)
- [ ] .env.production has chmod 600
- [ ] .env.production NOT committed to Git
- [ ] Backup of secrets stored securely offline
- [ ] Rotation schedule documented
- [ ] Team trained on secret management
- [ ] Incident response plan documented

## 🆘 Emergency Contacts

| Role | Contact | Use Case |
|------|---------|----------|
| DevOps Lead | devops@tucentropdf.com | Secret rotation, deploy issues |
| Security Team | security@tucentropdf.com | Compromise, audit |
| CTO | cto@tucentropdf.com | Critical incidents |

## 📚 Additional Resources

- [OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)
- [NIST Guidelines on Cryptographic Key Management](https://csrc.nist.gov/publications/detail/sp/800-57-part-1/rev-5/final)
- [Docker Secrets](https://docs.docker.com/engine/swarm/secrets/)

---

**Last Updated**: November 2025  
**Version**: 1.0  
**Owner**: DevOps Team
