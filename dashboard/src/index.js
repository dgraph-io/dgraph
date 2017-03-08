import React from "react";
import ReactDOM from "react-dom";
import { compose, createStore, applyMiddleware } from "redux";
import { Provider } from "react-redux";
import createLogger from "redux-logger";
import thunk from "redux-thunk";
import { persistStore, autoRehydrate } from "redux-persist";
import reducer from "./reducers";
import App from "./containers/App";

import "bootstrap/dist/css/bootstrap.css";
import "bootstrap/dist/css/bootstrap-theme.css";

const middleware = [thunk];
if (process.env.NODE_ENV !== "production") {
    middleware.push(createLogger());
}

const store = createStore(
    reducer,
    undefined,
    compose(applyMiddleware(...middleware), autoRehydrate()),
);

// begin periodically persisting the store
persistStore(store, { whitelist: "queries" });

ReactDOM.render(
    <Provider store={store}>
        <App />
    </Provider>,
    document.getElementById("root"),
);

// if (module.hot) {
//     module.hot.accept("./containers/App", () => {
//         const NextApp = require("./containers/App").default;
//         ReactDOM.render(<NextApp />, document.getElementById("root"));
//     });
// }
