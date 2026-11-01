# syntax=docker/dockerfile:1

# ---- Dependencies ----
FROM node:22-alpine AS deps
RUN corepack enable
WORKDIR /app

COPY package.json pnpm-lock.yaml ./
# Reuse the pnpm content-addressable store across builds.
RUN --mount=type=cache,id=pnpm,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir /pnpm/store

# ---- Builder ----
FROM node:22-alpine AS builder
RUN corepack enable
WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# NEXT_PUBLIC_* vars are inlined at build time
ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ARG NEXT_PUBLIC_BOT_USERNAME
ENV NEXT_PUBLIC_BOT_USERNAME=$NEXT_PUBLIC_BOT_USERNAME

RUN pnpm build

# ---- Runner ----
FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production

# Create non-root user
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# Copy standalone output
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000
ENV HOSTNAME="0.0.0.0"
ENV PORT=3000

# /login is public (no auth cookie needed) and exercises middleware + SSR.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD node -e "fetch('http://127.0.0.1:3000/login').then((r)=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

CMD ["node", "server.js"]
