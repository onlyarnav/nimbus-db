FROM node:20-alpine AS runner
RUN adduser -D -u 10001 appuser
WORKDIR /home/appuser
ENV NODE_ENV=production
COPY services/dashboard/public ./public
COPY services/dashboard/.next/standalone ./
COPY services/dashboard/.next/static ./.next/static
USER appuser
EXPOSE 3000
ENV PORT=3000
CMD ["node", "server.js"]

