# Dgraph User Interface

This project was bootstrapped with [Create React App](https://github.com/facebookincubator/create-react-app).

## Setting up

The following steps would help setup the app for development locally.

1. Make sure you have [Node.js](https://nodejs.org/en/) installed.
2. [Install and run](https://wiki.dgraph.io/Get_Started) Dgraph on the default port(8080) so that the frontend can communicate with it.
3. We use [npm](https://www.npmjs.com/) for dependency management. Run `npm install` from within the dashboard folder to install the deps.
4. Run `npm start` which would open up the UI at `http://localhost:3000`.The UI gets refreshed automatically after a change in any files inside the src folder.

You can run `npm run build` to generate the bundled files for production and push them too with your development changes. These files are served by Dgraph on the default port at `http://localhost:8080`.