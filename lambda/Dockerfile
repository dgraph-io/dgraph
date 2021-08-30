FROM node:14-alpine as build

RUN apk add python make g++
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm install

COPY . .
ARG nodeEnv=production
ENV NODE_ENV $nodeEnv
RUN npm run build && if [[ "$nodeEnv" == "production" ]]; then mv node_modules/node-webcrypto-ossl tmp && rm -rf node_modules && mkdir node_modules && mv tmp node_modules/node-webcrypto-ossl && npm install --no-optional; fi

# Used just for tests
ENTRYPOINT [ "npm", "run" ]

FROM node:14-alpine
ENV NODE_ENV production
RUN adduser app -h /app -D
USER app
WORKDIR /app
COPY --from=build --chown=app /app /app
CMD ["npm", "start"]
