// @flow

import React, { Component } from 'react';

class Label extends Component {

  render() {
    return (
		<div style={{display: 'inline-block'}}>
			<div className="App-label" style={{borderBottom: `5px solid ${this.props.color}`}}>{this.props.label}</div>
			<div style={{"display":"inline-block"}}>: {this.props.pred}</div>
		</div>
    )
  }
}

export default Label;
