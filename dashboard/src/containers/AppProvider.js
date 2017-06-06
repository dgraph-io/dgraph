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
import { incrementVisitCount, answeredNPSSurvey } from "../actions/user";

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
    persistStore(store, { whitelist: ["frames", "user"] }, () => {
      this.setState({ rehydrated: true }, this.onRehydrated);
    });
  }

  onRehydrated = () => {
    const currentState = store.getState();

    // Increment visit count
    store.dispatch(incrementVisitCount());

    // Collapse all except the first one to avoid slow render
    const frameItems = currentState.frames.items;
    if (frameItems.length > 0) {
      const firstFrame = frameItems[0];
      store.dispatch(toggleCollapseFrame(firstFrame, false));

      for (let i = 1; i < frameItems.length; i++) {
        const targetFrame = frameItems[i];

        store.dispatch(toggleCollapseFrame(targetFrame, true));
      }
    }

    // If NPS Survey is not done, show after a prolonged session
    if (!currentState.user.NPSSurveyDone) {
      // 30 minutes
      const surveyDelay = 1800000;

      setTimeout(() => {
        if (!store.getState().user.NPSSurveyDone) {
          /* global delighted */
          delighted.survey();
          store.dispatch(answeredNPSSurvey());
        }
      }, surveyDelay);
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
