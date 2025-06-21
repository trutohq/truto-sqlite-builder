# Multi-stage Dockerfile for truto-sqlite-builder
FROM node:20-alpine AS base

# Set working directory
WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci --only=production && npm cache clean --force

# Development stage
FROM node:20-alpine AS development

WORKDIR /app

# Install development dependencies
COPY package*.json ./
RUN npm ci && npm cache clean --force

# Copy source code
COPY . .

# Set environment for development
ENV NODE_ENV=development

# Expose port for potential dev server
EXPOSE 3000

# Default command for development
CMD ["npm", "run", "dev"]

# Test stage
FROM development AS test

# Run tests
RUN npm run lint
RUN npm run typecheck
RUN npm test
RUN npm run build

# Production stage  
FROM base AS production

# Copy built files
COPY --from=test /app/dist ./dist
COPY --from=test /app/package.json ./
COPY --from=test /app/README.md ./
COPY --from=test /app/LICENSE ./
COPY --from=test /app/CHANGELOG.md ./

# Set environment for production
ENV NODE_ENV=production

# Create non-root user
RUN addgroup -g 1001 -S nodejs
RUN adduser -S nodejs -u 1001

# Change to non-root user
USER nodejs

# Default command
CMD ["node", "--version"]

# Example usage stage
FROM node:20-alpine AS example

WORKDIR /app

# Install better-sqlite3 for examples
RUN npm install better-sqlite3

# Copy built package
COPY --from=production /app ./node_modules/truto-sqlite-builder/

# Copy example files
COPY examples/ ./examples/

# Default command to run examples
CMD ["node", "examples/basic.js"] 