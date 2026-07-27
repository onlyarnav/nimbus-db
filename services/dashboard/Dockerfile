FROM node:20-alpine AS builder
WORKDIR /app
COPY services/dashboard/package*.json ./
RUN npm ci
COPY services/dashboard/ ./
RUN npm run build

FROM node:20-alpine AS runner
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
ENV NODE_ENV=production
COPY --from=builder /app/public ./public
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
USER appuser
EXPOSE 3000
ENV PORT=3000
CMD ["node", "server.js"]
