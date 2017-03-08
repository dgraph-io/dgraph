import React, { Component } from "react";
import { connect } from "react-redux";

import ResponseInfo from "../components/ResponseInfo";

const mapStateToProps = state => ({
    numNodes: state.response.numNodes,
    numEdges: state.response.numEdges,
    latency: state.response.latency,
    rendering: state.response.rendering,
    treeView: state.response.treeView,
    // currentNode: state.interaction.currentNode,
});

export default connect(mapStateToProps, null)(ResponseInfo);
