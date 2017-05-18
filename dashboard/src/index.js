import React from "react";
import ReactDOM from "react-dom";

import AppProvider from "./containers/AppProvider";
import App from "./containers/App";

const render = Component => {
  return ReactDOM.render(
    <AppProvider component={Component} />,
    document.getElementById("root")
  );
};

render(App);

if (module.hot) {
    module.hot.accept("./containers/App", () => {
        const NextApp = require("./containers/App").default;
        render(NextApp);
    });
}
