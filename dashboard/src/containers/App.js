import React from "react";
import { connect } from "react-redux";
import screenfull from "screenfull";

import ScratchpadContainer from "../containers/ScratchpadContainer";
import NavBar from "../components/Navbar";
import PreviousQueryListContainer from "./PreviousQueryListContainer";
import Editor from "./Editor";
import Response from "./Response";
import { updateFullscreen } from "../actions";

import "../assets/css/App.css";

class App extends React.Component {
  enterFullScreen = updateFullscreen => {
    if (!screenfull.enabled) {
      return;
    }

    document.addEventListener(screenfull.raw.fullscreenchange, () => {
      updateFullscreen(screenfull.isFullscreen);
    });
    screenfull.request(document.getElementById("response"));
  };

  render = () => {
    return (
      <div>
        <NavBar />
        <div className="container-fluid">
          <div className="row justify-content-md-center">
            <div className="col-sm-12">
              <div className="col-sm-5">
                <Editor />
                <ScratchpadContainer />
                <PreviousQueryListContainer xs="hidden-xs" />
              </div>
              <div className="col-sm-7">
                <label style={{ marginLeft: "5px" }}> Response </label>
                {screenfull.enabled &&
                  <div
                    title="Enter full screen mode"
                    className="pull-right App-fullscreen"
                    onClick={() => this.enterFullScreen(this.props.updateFs)}
                  >
                    <span
                      className="App-fs-icon glyphicon glyphicon-glyphicon glyphicon-resize-full"
                    />
                  </div>}
                <Response />
                <PreviousQueryListContainer xs="visible-xs-block" />
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };
}

const mapDispatchToProps = dispatch => ({
  updateFs: fs => {
    dispatch(updateFullscreen(fs));
  }
});

export default connect(null, mapDispatchToProps)(App);
