import React from "react";
import classnames from "classnames";
import vis from "vis";
import { connect } from "react-redux";

import SessionGraphTab from "./SessionGraphTab";
import FrameCodeTab from "./FrameCodeTab";
import SessionTreeTab from "./SessionTreeTab";
import SessionFooter from "./SessionFooter";
import EntitySelector from "./EntitySelector";
import GraphIcon from "./GraphIcon";
import TreeIcon from "./TreeIcon";
import { getNodeLabel, shortenName } from "../lib/graph";

import { updatePreferredSessionTab } from "../actions/user";

class FrameSession extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      // tabs: query, graph, tree, json
      currentTab: props.preferredSessionTab,
      graphRenderStart: null,
      graphRenderEnd: null,
      treeRenderStart: null,
      treeRenderEnd: null,
      selectedNode: null,
      hoveredNode: null,
      isTreePartial: false,
      configuringNodeType: null,
      labelRegexText: ""
    };

    const { session: { response } } = props;
    this.nodes = new vis.DataSet(response.nodes);
    this.edges = new vis.DataSet(response.edges);
  }

  handleUpdateLabelRegexText = val => {
    this.setState({ labelRegexText: val });
  };

  handleBeforeGraphRender = () => {
    this.setState({ graphRenderStart: new Date() });
  };

  handleGraphRendered = () => {
    this.setState({ graphRenderEnd: new Date() });
  };

  handleBeforeTreeRender = () => {
    this.setState({ treeRenderStart: new Date() });
  };

  handleTreeRendered = () => {
    this.setState({ treeRenderEnd: new Date() });
  };

  handleNodeSelected = node => {
    if (!node) {
      this.setState({
        selectedNode: null,
        hoveredNode: null,
        configuringNodeType: null
      });
      return;
    }

    this.setState({ selectedNode: node });
  };

  handleNodeHovered = node => {
    this.setState({ hoveredNode: node });
  };

  handleInitNodeTypeConfig = nodeType => {
    const { configuringNodeType } = this.state;

    let nextValue;
    if (configuringNodeType === nodeType) {
      nextValue = "";
    } else {
      nextValue = nodeType;
    }
    this.setState({ configuringNodeType: nextValue });
  };

  navigateTab = (tabName, e) => {
    const { handleSetPreferredSessionTab } = this.props;
    e.preventDefault();

    this.setState({ currentTab: tabName }, () => {
      handleSetPreferredSessionTab(tabName);
    });
  };

  getGraphRenderTime = () => {
    const { graphRenderStart, graphRenderEnd } = this.state;
    if (!graphRenderStart || !graphRenderEnd) {
      return;
    }

    return graphRenderEnd.getTime() - graphRenderStart.getTime();
  };

  getTreeRenderTime = () => {
    const { treeRenderStart, treeRenderEnd } = this.state;
    if (!treeRenderStart || !treeRenderEnd) {
      return;
    }

    return treeRenderEnd.getTime() - treeRenderStart.getTime();
  };

  handleUpdateLabels = () => {
    const { labelRegexText } = this.state;
    const re = new RegExp(labelRegexText);

    this.applyLabels(this.nodes, re);
  };

  /**
   * applyLabels applies labels to a set of nodes, given a regex object for labels
   * @params nodeSet {vis.DataSet} - a vis.js dataset holding nodes
   * @params labelRegex {RegExp} - regex for labels
   *
   * mutates the nodeSet
   */
  applyLabels = (nodeSet, labelRegex) => {
    const allNodes = nodeSet.get();
    const updatedNodes = allNodes.map(node => {
      const properties = JSON.parse(node.title);
      const fullName = getNodeLabel(properties.attrs, labelRegex);
      const displayLabel = shortenName(fullName);

      return Object.assign({}, node, {
        label: displayLabel
      });
    });

    nodeSet.update(updatedNodes);
  };

  render() {
    const { session } = this.props;
    const {
      currentTab,
      selectedNode,
      hoveredNode,
      configuringNodeType,
      isConfiguringLabel,
      labelRegexText
    } = this.state;

    return (
      <div className="body">
        <div className="content">
          <div className="sidebar">
            <ul className="sidebar-nav">
              <li>
                <a
                  href="#graph"
                  className={classnames("sidebar-nav-item", {
                    active: currentTab === "graph"
                  })}
                  onClick={this.navigateTab.bind(this, "graph")}
                >
                  <div className="icon-container">
                    <GraphIcon />
                  </div>
                  <span className="menu-label">
                    Graph
                  </span>
                </a>
              </li>
              <li>
                <a
                  href="#tree"
                  className={classnames("sidebar-nav-item", {
                    active: currentTab === "tree"
                  })}
                  onClick={this.navigateTab.bind(this, "tree")}
                >
                  <div className="icon-container">
                    <TreeIcon />
                  </div>
                  <span className="menu-label">Tree</span>

                </a>
              </li>
              <li>
                <a
                  href="#tree"
                  className={classnames("sidebar-nav-item", {
                    active: currentTab === "json"
                  })}
                  onClick={this.navigateTab.bind(this, "json")}
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
            {currentTab === "graph"
              ? <SessionGraphTab
                  session={session}
                  onBeforeGraphRender={this.handleBeforeGraphRender}
                  onGraphRendered={this.handleGraphRendered}
                  onNodeSelected={this.handleNodeSelected}
                  onNodeHovered={this.handleNodeHovered}
                  nodesDataset={this.nodes}
                  edgesDataset={this.edges}
                  labelRegexText={labelRegexText}
                  applyLabels={this.applyLabels}
                />
              : null}

            {currentTab === "tree"
              ? <SessionTreeTab
                  session={session}
                  onBeforeTreeRender={this.handleBeforeTreeRender}
                  onTreeRendered={this.handleTreeRendered}
                  onNodeSelected={this.handleNodeSelected}
                  onNodeHovered={this.handleNodeHovered}
                  selectedNode={selectedNode}
                  nodesDataset={this.nodes}
                  edgesDataset={this.edges}
                  labelRegexText={labelRegexText}
                  applyLabels={this.applyLabels}
                />
              : null}

            {currentTab === "json"
              ? <FrameCodeTab
                  query={session.query}
                  response={session.response.data}
                />
              : null}

            {currentTab === "graph" || currentTab === "tree"
              ? <EntitySelector
                  response={session.response}
                  onInitNodeTypeConfig={this.handleInitNodeTypeConfig}
                  labelRegexText={labelRegexText}
                  onUpdateLabelRegexText={this.handleUpdateLabelRegexText}
                  onUpdateLabels={this.handleUpdateLabels}
                />
              : null}

            <SessionFooter
              session={session}
              currentTab={currentTab}
              selectedNode={selectedNode}
              hoveredNode={hoveredNode}
              configuringNodeType={configuringNodeType}
              graphRenderTime={this.getGraphRenderTime()}
              treeRenderTime={this.getTreeRenderTime()}
              isConfiguringLabel={isConfiguringLabel}
            />
          </div>
        </div>
      </div>
    );
  }
}

const mapStateToProps = state => ({
  preferredSessionTab: state.user.preferredSessionTab
});

const mapDispatchToProps = dispatch => ({
  handleSetPreferredSessionTab(tabName) {
    dispatch(updatePreferredSessionTab(tabName));
  }
});

export default connect(mapStateToProps, mapDispatchToProps)(FrameSession);
