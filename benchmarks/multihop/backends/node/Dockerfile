FROM node:20-alpine
WORKDIR /app
COPY package.json server.js ./
EXPOSE 9101
ENV PORT=9101
CMD ["node", "server.js"]
