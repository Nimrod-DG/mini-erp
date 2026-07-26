# The web image: build the bundle, serve it as static files.
#
# THE VITE_* VALUES ARE BAKED IN AT BUILD TIME, not read at runtime. That is how
# Vite works — `import.meta.env.X` is substituted into the bundle — so this image
# is specific to the Firebase project and API origin it was built against, and
# Phase 9 builds a second one for prod rather than re-pointing this one.
#
# They are also not secrets. The Firebase web config is a set of public
# identifiers that ship inside every client bundle by design; `.env.example` says
# so. The defaults below are placeholders so CI can build the image without
# credentials, and a real build passes --build-arg for each.

FROM node:22-alpine AS build

WORKDIR /app

# npm ci, not npm install: it installs exactly the lockfile, which is the only
# way two builds of one commit produce the same bundle.
COPY package.json package-lock.json ./
RUN npm ci

COPY . .

ARG VITE_FIREBASE_API_KEY=placeholder
ARG VITE_FIREBASE_AUTH_DOMAIN=placeholder.firebaseapp.com
ARG VITE_FIREBASE_PROJECT_ID=placeholder
ARG VITE_FIREBASE_APP_ID=placeholder
ARG VITE_API_BASE_URL=http://localhost:8080

ENV VITE_FIREBASE_API_KEY=$VITE_FIREBASE_API_KEY \
    VITE_FIREBASE_AUTH_DOMAIN=$VITE_FIREBASE_AUTH_DOMAIN \
    VITE_FIREBASE_PROJECT_ID=$VITE_FIREBASE_PROJECT_ID \
    VITE_FIREBASE_APP_ID=$VITE_FIREBASE_APP_ID \
    VITE_API_BASE_URL=$VITE_API_BASE_URL

# `tsc -b && vite build`. The typecheck is part of the build deliberately: an
# image that compiles but does not typecheck is an image nobody checked.
RUN npm run build

FROM nginx:1.29-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
