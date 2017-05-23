import React from "react";
import classnames from "classnames";

import FrameCodeTab from "./FrameCodeTab";
import FrameMessageTab from "./FrameMessageTab";

class FrameError extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      // tabs: 'error', 'response', 'query'
      currentTab: "error"
    };
  }

  navigateTab = (tabName, e) => {
    e.preventDefault();

    this.setState({
      currentTab: tabName
    });
  };

  render() {
    const { data: { message, response, query } } = this.props;
    const { currentTab } = this.state;

    return (
      <div className="body">
        <div className="content">
          <div className="sidebar">
            <ul className="sidebar-nav">
              <li>
                <a
                  href="#query"
                  className={classnames("sidebar-nav-item", {
                    active: currentTab === "error"
                  })}
                  onClick={this.navigateTab.bind(this, "error")}
                >
                  <div className="icon-container">
                    <i className="icon fa fa-warning" />
                  </div>
                  <span className="menu-label">Error</span>

                </a>
              </li>
              <li>
                <a
                  href="#tree"
                  className={classnames("sidebar-nav-item", {
                    active: currentTab === "response"
                  })}
                  onClick={this.navigateTab.bind(this, "response")}
                >
                  <div className="icon-container">
                    <i className="icon fa fa-code" />
                  </div>

                  <span className="menu-label">JSON</span>

                </a>
              </li>
            </ul>
          </div>

          <div className="main">
            {currentTab === "error"
              ? <FrameMessageTab message={message} />
              : null}
            {currentTab === "response"
              ? <FrameCodeTab query={query} response={response} />
              : null}

            <div className="footer error-footer">
              <i className="fa fa-warning error-mark" />
              {" "}
              <span className="result-message">Error occurred</span>
            </div>
          </div>
        </div>
      </div>
    );
  }
}

export default FrameError;
