import React from 'react';
import classnames from 'classnames';

import logo from "../assets/images/dgraph.png";

class Sidebar extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      currentMenu: ''
    };
  }

  handleToggleMenu = (targetMenu) => {
    const { currentMenu } = this.state;

    let nextState = '';
    if (currentMenu !== targetMenu) {
      nextState = targetMenu;
    }

    this.setState({
      currentMenu: nextState
    });
  }

  render() {
    const { currentMenu } = this.state;

    return (
      <div className="sidebar-container">
        <div className="sidebar-menu">
          <ul>
            <li className="brand">
              <a
                href="#"
                className={classnames('link', { active: currentMenu === 'about' })}
                onClick={(e) => {
                  e.preventDefault();
                  this.handleToggleMenu('about')
                }}
              >
                <img src={logo} alt="logo" className="logo" />
              </a>
            </li>
            {
              /*
              <li>
                <a
                  href="#"
                  className={classnames('link', { active: currentMenu === 'favorite' })}
                  onClick={(e) => {
                    e.preventDefault();
                    this.handleToggleMenu('favorite')
                  }}
                >
                  <i className="fa fa-star" />
                </a>
              </li>
              */
            }
            <li>
              <a
                href="#"
                className={classnames('link', { active: currentMenu === 'help' })}
                onClick={(e) => {
                  e.preventDefault();
                  this.handleToggleMenu('help')
                }}
              >
                <i className="fa fa-question-circle-o" />
              </a>
            </li>
          </ul>
        </div>
        <div className={classnames('sidebar-content', { open: Boolean(currentMenu) })}>
          {currentMenu === 'favorite' ?
            <div>favorite</div> : null}
          {currentMenu === 'about' ?
            <div>about</div> : null}
          {currentMenu === 'help' ?
            <div>help</div> : null}
        </div>
      </div>
    );
  }
}

export default Sidebar;
