import React from "react";
import ReactDOM from "react-dom";
import { compose, createStore, applyMiddleware } from "redux";
import { Provider } from "react-redux";
import thunk from "redux-thunk";
import { persistStore, autoRehydrate } from "redux-persist";
import reducer from "./reducers";
import App from "./containers/App";

import "bootstrap/dist/css/bootstrap.css";
import "bootstrap/dist/css/bootstrap-theme.css";

const middleware = [thunk];

const composeEnhancers = window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__ || compose;

const store = createStore(
    reducer,
    undefined,
    composeEnhancers(applyMiddleware(...middleware), autoRehydrate()),
);

// begin periodically persisting the store
persistStore(store, { whitelist: "previousQueries, query" });

const render = Component => {
    return ReactDOM.render(
        <Provider store={store}>
            <Component />
        </Provider>,
        document.getElementById("root"),
    );
};

render(App);

if (module.hot) {
    module.hot.accept("./containers/App", () => {
        const NextApp = require("./containers/App").default;
        render(NextApp);
    });
}
