import React from "react";
import classnames from "classnames";

import logo from "../assets/images/dgraph.png";
import SidebarInfo from "./SidebarInfo";
import SidebarFeedback from "./SidebarFeedback";

import "../assets/css/Sidebar.css";

class Sidebar extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      rotate: false
    };
  }
  render() {
    const { currentMenu, onToggleMenu } = this.props;
    const { rotate } = this.state;

    return (
      <div className="sidebar-container">
        <div className="sidebar-menu">
          <ul>
            <li className="brand">
              <a
                href="#"
                className={classnames("link", {
                  active: currentMenu === "about"
                })}
                onClick={e => {
                  e.preventDefault();
                  this.setState({ rotate: !rotate });
                }}
              >
                <img
                  src={logo}
                  alt="logo"
                  className={classnames("logo", { rotate })}
                />
              </a>
            </li>
            {/*
              <li>
                <a
                  href="#"
                  className={classnames('link', { active: currentMenu === 'favorite' })}
                  onClick={(e) => {
                    e.preventDefault();
                    onToggleMenu('favorite')
                  }}
                >
                  <i className="fa fa-star" />
                </a>
              </li>
              */}
            <li>
              <a
                href="#info"
                className={classnames("link", {
                  active: currentMenu === "info"
                })}
                onClick={e => {
                  e.preventDefault();
                  onToggleMenu("info");
                }}
              >
                <i className="fa fa-question-circle-o" />
              </a>
            </li>
            <li>
              <a
                href="#info"
                className={classnames("link", {
                  active: currentMenu === "feedback"
                })}
                onClick={e => {
                  e.preventDefault();
                  onToggleMenu("feedback");
                }}
              >
                <i className="fa fa-commenting" />
              </a>
            </li>
          </ul>
        </div>
        <div
          className={classnames("sidebar-content", {
            open: Boolean(currentMenu)
          })}
        >
          {currentMenu === "favorite" ? <div>favorite</div> : null}
          {currentMenu === "about" ? <div>about</div> : null}
          {currentMenu === "info" ? <SidebarInfo /> : null}
          {currentMenu === "feedback" ? <SidebarFeedback /> : null}
        </div>
      </div>
    );
  }
}

export default Sidebar;
