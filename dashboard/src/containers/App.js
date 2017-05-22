import React from "react";
import { connect } from "react-redux";

import Sidebar from "../components/Sidebar";
import EditorPanel from "../components/EditorPanel";
import FrameList from "../components/FrameList";
import { runQuery, runQueryByShareId } from "../actions";
import { refreshConnectedState } from "../actions/connection";
import {
  discardFrame,
  discardAllFrames,
  toggleCollapseFrame
} from "../actions/frames";
import { createCookie, readCookie, eraseCookie } from "../lib/helpers";

import "../assets/css/App.css";

class App extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      query: "",
      isQueryDirty: false,
      currentSidebarMenu: "",
      // queryExecutionCounter is used to determine when the NPS score survey
      // should be shown
      queryExecutionCounter: 0
    };
  }

  componentDidMount = () => {
    const { handleRunQuery, _refreshConnectedState, match } = this.props;

    _refreshConnectedState();

    const { shareId } = match.params;
    if (shareId) {
      this.onRunSharedQuery(shareId);
    }

    // If playQuery cookie is set, run the query and erase the cookie
    // The cookie is used to communicate the query string between docs and play
    const playQuery = readCookie("playQuery");
    if (playQuery) {
      const queryString = decodeURI(playQuery);
      handleRunQuery(queryString).then(() => {
        eraseCookie("playQuery", { crossDomain: true });
      });
    }
  };

  handleToggleSidebarMenu = targetMenu => {
    const { currentSidebarMenu } = this.state;

    let nextState = "";
    if (currentSidebarMenu !== targetMenu) {
      nextState = targetMenu;
    }

    this.setState({
      currentSidebarMenu: nextState
    });
  };

  // saveCodeMirrorInstance saves the codemirror instance initialized in the
  // <Editor /> component so that we can access it in this component. (e.g. to
  // focus)
  saveCodeMirrorInstance = codemirror => {
    this._codemirror = codemirror;
  };

  handleUpdateQuery = (val, done = () => {}) => {
    const isQueryDirty = val.trim() !== "";

    this.setState({ query: val, isQueryDirty }, done);
  };

  // focusCodemirror sets focus on codemirror and moves the cursor to the end
  focusCodemirror = () => {
    const cm = this._codemirror;
    const lastlineNumber = cm.doc.lastLine();
    const lastCharPos = cm.doc.getLine(lastlineNumber).length;

    cm.focus();
    cm.setCursor({ line: lastlineNumber, ch: lastCharPos });
  };

  handleSelectQuery = val => {
    this.handleUpdateQuery(val, this.focusCodemirror);
  };

  handleClearQuery = () => {
    this.handleUpdateQuery("", this.focusCodemirror);
  };

  collapseAllFrames = () => {
    const { frames, handleCollapseFrame } = this.props;

    frames.forEach(handleCollapseFrame);
  };

  handleRunQuery = query => {
    const { _handleRunQuery } = this.props;

    // First, collapse all frames in order to prevent slow rendering
    // FIXME: this won't be necessary if visualization took up less resources
    // TODO: Compare benchmarks between d3.js and vis.js and make migration if needed
    this.collapseAllFrames();

    _handleRunQuery(query, () => {
      const { queryExecutionCounter } = this.state;

      if (queryExecutionCounter === 5) {
        if (!readCookie("nps-survery-done")) {
          /* global delighted */
          delighted.survey();
          createCookie("nps-survery-done", true, 180);
        }
      } else {
        this.setState({ queryExecutionCounter: queryExecutionCounter + 1 });
      }
    });
  };

  handleDiscardAllFrames = () => {
    const { _handleDiscardAllFrames } = this.props;

    _handleDiscardAllFrames();
  };

  onRunSharedQuery = shareId => {
    const { handleRunSharedQuery } = this.props;

    handleRunSharedQuery(shareId).catch(e => {
      console.log(e);
    });
  };

  render = () => {
    const { query, isQueryDirty, currentSidebarMenu } = this.state;
    const { handleDiscardFrame, frames, connected } = this.props;

    const canDiscardAll = frames.length > 0;

    return (
      <div className="app-layout">
        <Sidebar
          currentMenu={currentSidebarMenu}
          onToggleMenu={this.handleToggleSidebarMenu}
        />
        <div className="main-content">
          {currentSidebarMenu !== ""
            ? <div
                className="click-capture"
                onClick={e => {
                  e.stopPropagation();
                  this.setState({
                    currentSidebarMenu: ""
                  });
                }}
              />
            : null}
          <div className="container-fluid">
            <div className="row justify-content-md-center">
              <div className="col-sm-12">
                <EditorPanel
                  query={query}
                  isQueryDirty={isQueryDirty}
                  canDiscardAll={canDiscardAll}
                  onDiscardAllFrames={this.handleDiscardAllFrames}
                  onRunQuery={this.handleRunQuery}
                  onUpdateQuery={this.handleUpdateQuery}
                  onClearQuery={this.handleClearQuery}
                  saveCodeMirrorInstance={this.saveCodeMirrorInstance}
                  connected={connected}
                />
              </div>

              <div className="col-sm-12">
                <FrameList
                  frames={frames}
                  onDiscardFrame={handleDiscardFrame}
                  onSelectQuery={this.handleSelectQuery}
                  collapseAllFrames={this.collapseAllFrames}
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };
}

const mapStateToProps = state => ({
  frames: state.frames.items,
  connected: state.connection.connected
});

const mapDispatchToProps = dispatch => ({
  _handleRunQuery(query, done = () => {}) {
    return dispatch(runQuery(query)).then(done);
  },
  _handleDiscardAllFrames() {
    return dispatch(discardAllFrames());
  },
  _refreshConnectedState() {
    dispatch(refreshConnectedState());
  },
  handleRunSharedQuery(shareId) {
    return dispatch(runQueryByShareId(shareId));
  },
  handleDiscardFrame(frameID) {
    dispatch(discardFrame(frameID));
  },
  handleCollapseFrame(frame) {
    dispatch(toggleCollapseFrame(frame, true));
  }
});

export default connect(mapStateToProps, mapDispatchToProps)(App);
