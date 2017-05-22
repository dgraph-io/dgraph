import React from "react";
import { Provider } from "react-redux";
import { compose, createStore, applyMiddleware } from "redux";
import { persistStore, autoRehydrate } from "redux-persist";
import {
  BrowserRouter as Router,
  Route,
  browserHistory
} from "react-router-dom";
import thunk from "redux-thunk";
import reducer from "../reducers";
import { toggleCollapseFrame } from "../actions/frames";

import "bootstrap/dist/css/bootstrap.css";
import "bootstrap/dist/css/bootstrap-theme.css";

const middleware = [thunk];
const composeEnhancers = window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__ || compose;
const store = createStore(
  reducer,
  undefined,
  composeEnhancers(applyMiddleware(...middleware), autoRehydrate())
);

export default class AppProvider extends React.Component {
  constructor() {
    super();
    this.state = {
      rehydrated: false
    };
  }

  componentWillMount() {
    // begin periodically persisting the store
    persistStore(store, { whitelist: ["frames"] }, () => {
      this.setState({ rehydrated: true }, this.onRehydrated);
    });
  }

  onRehydrated = () => {
    const currentState = store.getState();
    const frameItems = currentState.frames.items;

    // Collapse all except the first one to avoid slow render
    const firstFrame = frameItems[0];
    store.dispatch(toggleCollapseFrame(firstFrame, false));

    for (let i = 1; i < frameItems.length; i++) {
      const targetFrame = frameItems[i];

      store.dispatch(toggleCollapseFrame(targetFrame, true));
    }
  };

  render() {
    const { component } = this.props;
    const { rehydrated } = this.state;

    if (!rehydrated) {
      return (
        <div>
          Loading
        </div>
      );
    }

    return (
      <Provider store={store}>
        <Router history={browserHistory}>
          <Route path="/:shareId?" component={component} />
        </Router>
      </Provider>
    );
  }
}
