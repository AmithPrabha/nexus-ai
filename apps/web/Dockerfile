FROM node:24-alpine AS build
WORKDIR /web
ENV NEXT_TELEMETRY_DISABLED=1
COPY apps/web/package*.json ./
RUN npm ci --no-fund
COPY apps/web/app ./app
COPY apps/web/lib ./lib
COPY apps/web/tsconfig.json apps/web/next.config.mjs apps/web/postcss.config.mjs apps/web/tailwind.config.cjs ./
COPY packages/contracts/openapi.json public/openapi.json
RUN npm run build

FROM node:24-alpine
WORKDIR /web
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
COPY --from=build /web/.next/standalone ./
COPY --from=build /web/.next/static ./.next/static
COPY --from=build /web/public ./public
USER node
CMD ["node","server.js"]
