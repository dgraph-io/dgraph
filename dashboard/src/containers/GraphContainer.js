import React, { Component } from "react";
import vis from "vis";
import { connect } from "react-redux";
import _ from "lodash/object";
import classnames from "classnames";

import { renderNetwork } from "../lib/graph";
import Progress from "../components/Progress";
import PartialRenderInfo from "../components/PartialRenderInfo";
import { outgoingEdges, childNodes } from "../lib/helpers";

import "../assets/css/Graph.css";
import "vis/dist/vis.min.css";

const doubleClickTime = 0;
const threshold = 200;

class GraphContainer extends Component {
  constructor(props: Props) {
    super(props);

    this.state = {
      renderProgress: 0,
      partiallyRendered: false
    };
  }

  componentDidMount() {
    const {
      response,
      treeView,
      onBeforeRender,
      onRendered,
      nodesDataset,
      edgesDataset
    } = this.props;

    onBeforeRender();

    const { network } = renderNetwork({
      nodes: nodesDataset,
      edges: edgesDataset,
      allNodes: response.allNodes,
      allEdges: response.allEdges,
      containerEl: this.refs.graph,
      treeView
    });

    // In tree view, physics is disabled and stabilizationIterationDone is not fired.
    if (treeView) {
      this.setState({ renderProgress: 100 }, () => {
        onRendered();
        // FIXME: tree does not fit because when it is rendered at the initial render, it is not visible
        // maybe lazy render
        network.fit();
      });
    }

    this.configNetwork(network);

    this.setState({ network }, () => {
      window.addEventListener("resize", this.fitNetwork);
    });

    // FIXME: hacky workaround for zoom problem: https://github.com/almende/vis/issues/3021
    const els = document.getElementsByClassName("vis-network");
    for (var i = 0; i < els.length; i++) {
      els[i].style.width = null;
    }
  }

  componentWillUnmount() {
    window.removeEventListener("resize", this.fitNetwork);
  }

  // fitNetwork update the fit of the network
  fitNetwork = () => {
    const { network } = this.state;

    if (network) {
      network.fit();
    }
  };

  // configNetwork configures the custom behaviors for a a network
  configNetwork = network => {
    const {
      response: { allNodes, allEdges },
      onNodeSelected,
      onNodeHovered
    } = this.props;
    const { data } = network.body;
    const allEdgeSet = new vis.DataSet(allEdges);
    const allNodeSet = new vis.DataSet(allNodes);

    if (
      allNodeSet.length !== data.nodes.length ||
      allEdgeSet.length !== data.edges.length
    ) {
      this.setState({ partiallyRendered: true });
    }

    // multiLevelExpand recursively expands all edges outgoing from the node
    function multiLevelExpand(nodeId) {
      let nodes = [nodeId], nodeStack = [nodeId], adjEdges = [];
      while (nodeStack.length !== 0) {
        let nodeId = nodeStack.pop();

        let outgoing = outgoingEdges(nodeId, allEdgeSet),
          adjNodeIds = outgoing.map(function(edge) {
            return edge.to;
          });

        nodeStack = nodeStack.concat(adjNodeIds);
        nodes = nodes.concat(adjNodeIds);
        adjEdges = adjEdges.concat(outgoing);
        if (adjNodeIds.length > 3) {
          break;
        }
      }
      data.nodes.update(allNodeSet.get(nodes));
      data.edges.update(adjEdges);
    }

    network.on("stabilizationProgress", params => {
      var widthFactor = params.iterations / params.total;

      this.setState({
        renderProgress: widthFactor * 100
      });
    });

    network.once("stabilizationIterationsDone", () => {
      const { onRendered } = this.props;
      this.setState({ renderProgress: 100 }, () => {
        network.fit();
        onRendered();
      });
    });

    network.on("click", params => {
      const t0 = new Date();

      if (t0 - doubleClickTime > threshold) {
        setTimeout(() => {
          if (t0 - doubleClickTime < threshold) {
            return;
          }

          if (params.nodes.length > 0) {
            const nodeUid = params.nodes[0];
            const clickedNode = data.nodes.get(nodeUid);

            onNodeSelected(clickedNode);
          } else if (params.edges.length > 0) {
            const edgeUid = params.edges[0];
            const currentEdge = data.edges.get(edgeUid);

            onNodeSelected(currentEdge);
          } else {
            onNodeSelected(null);
          }
        }, threshold);
      }
    });

    network.on("doubleClick", params => {
      if (params.nodes && params.nodes.length > 0) {
        const clickedNodeUid = params.nodes[0];
        const clickedNode = data.nodes.get(clickedNodeUid);

        network.unselectAll();
        onNodeSelected(clickedNode);

        const outgoing = outgoingEdges(clickedNodeUid, data.edges);
        const allOutgoingEdges = outgoingEdges(clickedNodeUid, allEdgeSet);
        const expanded = outgoing.length > 0 || allOutgoingEdges.length === 0;

        let adjacentNodeIds: Array<string> = allOutgoingEdges.map(function(
          edge
        ) {
          return edge.to;
        });

        let adjacentNodes = allNodeSet.get(adjacentNodeIds);

        // TODO -See if we can set a meta property to a node to know that its
        // expanded or closed and avoid this computation.
        if (expanded) {
          // Collapse all child nodes recursively.
          let allEdges = outgoing.map(function(edge) {
            return edge.id;
          });

          let allNodes = adjacentNodes.slice();

          while (adjacentNodeIds.length > 0) {
            let node = adjacentNodeIds.pop();
            let connectedEdges = outgoingEdges(node, data.edges);

            let connectedNodes = connectedEdges.map(function(edge) {
              return edge.to;
            });

            allNodes = allNodes.concat(connectedNodes);
            allEdges = allEdges.concat(connectedEdges);
            adjacentNodeIds = adjacentNodeIds.concat(connectedNodes);
          }

          data.nodes.remove(allNodes);
          data.edges.remove(allEdges);
        } else {
          multiLevelExpand(clickedNodeUid);
          if (data.nodes.length === allNodeSet.length) {
            this.setState({ partiallyRendered: false });
          }
        }
      }
    });

    network.on("hoverNode", params => {
      const nodeUID: string = params.node;
      const currentNode = data.nodes.get(nodeUID);

      onNodeHovered(currentNode);
    });

    network.on("blurNode", params => {
      onNodeHovered(null);
    });

    network.on("hoverEdge", params => {
      const edgeUID = params.edge;
      const currentEdge = data.edges.get(edgeUID);

      onNodeHovered(currentEdge);
    });

    network.on("blurEdge", params => {
      onNodeHovered(null);
    });

    network.on("dragEnd", function(params) {
      for (let i = 0; i < params.nodes.length; i++) {
        let nodeId: string = params.nodes[i];
        data.nodes.update({ id: nodeId, fixed: { x: true, y: true } });
      }
    });

    network.on("dragStart", function(params) {
      for (let i = 0; i < params.nodes.length; i++) {
        let nodeId: string = params.nodes[i];
        data.nodes.update({ id: nodeId, fixed: { x: false, y: false } });
      }
    });
  };

  // collapse the network
  handleCollapseNetwork = e => {
    e.preventDefault();

    const { network, partiallyRendered } = this.state;
    const { response: { nodes, edges } } = this.props;

    const { data } = network.body;

    if (partiallyRendered) {
      return;
    }

    data.nodes.remove(data.nodes.getIds());
    data.edges.remove(data.edges.getIds());
    // Since we don't mutate the nodes and edges passed as props initially,
    // this still holds the initial state that was rendered and we can collapse
    // back the graph to that state.
    data.nodes.update(nodes);
    data.edges.update(edges);
    this.setState({ partiallyRendered: true });
    network.fit();
  };

  handleExpandNetwork = () => {
    const { response: { allNodes, allEdges } } = this.props;
    const { network } = this.state;

    const { data } = network.body;
    const allEdgeSet = new vis.DataSet(allEdges);
    const allNodeSet = new vis.DataSet(allNodes);

    let nodeIds = data.nodes.getIds(),
      nodeSet = data.nodes,
      edgeSet = data.edges,
      // We add nodes and edges that have to be updated to these arrays.
      nodesBatch = new Set(),
      edgesBatch = [],
      batchSize = 500;

    while (nodeIds.length > 0) {
      let nodeId = nodeIds.pop();
      // If is expanded, do nothing, else put child nodes and edges into array for
      // expansion.
      if (
        outgoingEdges(nodeId, edgeSet).length ===
        outgoingEdges(nodeId, allEdgeSet).length
      ) {
        continue;
      }

      let outEdges = outgoingEdges(nodeId, allEdgeSet),
        outNodeIds = childNodes(outEdges);

      nodeIds = nodeIds.concat(outNodeIds);

      for (let id of outNodeIds) {
        nodesBatch.add(id);
      }

      edgesBatch = edgesBatch.concat(outEdges);

      if (nodesBatch.size > batchSize) {
        nodeSet.update(allNodeSet.get(Array.from(nodesBatch)));
        edgeSet.update(edgesBatch);
        nodesBatch = new Set();
        edgesBatch = [];
        return;
      }
    }

    if (nodeIds.length === 0) {
      this.setState({ partiallyRendered: false });
    }

    if (nodesBatch.size > 0 || edgesBatch.length > 0) {
      nodeSet.update(allNodeSet.get(Array.from(nodesBatch)));
      edgeSet.update(edgesBatch);
    }

    network.fit();
  };

  render() {
    const { response } = this.props;
    const { renderProgress, partiallyRendered } = this.state;

    const isRendering = renderProgress !== 100;
    const canToggleExpand =
      response.nodes.length !== response.numNodes &&
      response.edges.length !== response.numEdges;

    return (
      <div className="graph-container">
        {!isRendering && canToggleExpand
          ? <PartialRenderInfo
              partiallyRendered={partiallyRendered}
              onExpandNetwork={this.handleExpandNetwork}
              onCollapseNetwork={this.handleCollapseNetwork}
            />
          : null}
        {isRendering ? <Progress perc={renderProgress} /> : null}
        <div
          ref="graph"
          className={classnames("graph", { hidden: isRendering })}
        />
      </div>
    );
  }
}

const mapStateToProps = state => ({});

export default connect(mapStateToProps, null, null, { withRef: true })(
  GraphContainer
);
