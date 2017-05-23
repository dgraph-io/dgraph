import React from "react";
import classnames from "classnames";

import "../assets/css/SessionFooterProperties.css";

class SessionFooterProperties extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      isExpanded: false,
      canExpand: false
    };
  }

  handleToggleExpand = e => {
    e.preventDefault();

    this.setState({
      isExpanded: !this.state.isExpanded
    });
  };

  // isEllipsisActive detects if .properties elements is overflown and hence
  // ellipsis is active
  isEllipsisActive = () => {
    const propertiesEl = this._properties;
    if (!propertiesEl) {
      return false;
    }

    return propertiesEl.clientWidth < propertiesEl.scrollWidth;
  };

  refreshCanExpand = () => {
    if (this.state.isExpanded) {
      return;
    }

    const canExpand = this.isEllipsisActive();

    if (!this.state.canExpand && canExpand) {
      this.setState({ canExpand: true });
    } else if (this.state.canExpand && !canExpand) {
      this.setState({ canExpand: false });
    }
  };

  componentDidMount() {
    this.refreshCanExpand();
  }

  componentDidUpdate() {
    this.refreshCanExpand();
  }

  render() {
    const { entity } = this.props;
    const { canExpand, isExpanded } = this.state;

    const nodeProperties = JSON.parse(entity.title);

    // Nodes have facets and attrs keys.
    const isNode = Object.keys(nodeProperties).length !== 1;
    const attrs = nodeProperties.attrs;
    const facets = nodeProperties.facets;
    const facetKeys = Object.keys(facets);

    return (
      <div className="properties-container">
        <div
          className={classnames("properties", { expanded: isExpanded })}
          ref={el => {
            this._properties = el;
          }}
        >
          <span className="label label-info">
            {isNode ? "Node" : "Edge"}
          </span>

          {isNode && attrs
            ? Object.keys(attrs).map(function(key, idx) {
                return (
                  <span className="property-pair" key={idx}>
                    <span className="property-key">
                      <span className="key-content">{key}</span>:
                    </span>
                    <span className="property-val">
                      {String(attrs[key])}
                    </span>
                  </span>
                );
              })
            : null}

          {isNode && facetKeys.length > 0
            ? <span>
                <span className="label label-default">
                  Facets
                </span>
                {Object.keys(facets).map(function(key, idx) {
                  return (
                    <span className="property-pair" key={idx}>
                      <span className="property-key">
                        <span className="key-content">{key}</span>:
                      </span>
                      <span className="property-val">
                        {String(facets[key])}
                      </span>
                    </span>
                  );
                })}
              </span>
            : null}
        </div>

        {canExpand
          ? <a
              href="#toggle-expand"
              onClick={this.handleToggleExpand}
              className="toggle-expand-btn"
            >
              {isExpanded
                ? <i className="fa fa-caret-up" />
                : <i className="fa fa-caret-down" />}
            </a>
          : null}
      </div>
    );
  }
}

export default SessionFooterProperties;
