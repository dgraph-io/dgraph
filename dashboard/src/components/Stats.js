import React, { Component } from 'react';

// TODO - Later have one css file per component.
import '../assets/css/App.css';

class Stats extends Component {
  render() {
    return (
      <div className={`App-stats ${this.props.class}`}>
      		<span>{this.props.latency !== '' && this.props.rendering !== '' ? '* Server Latency - ' + this.props.latency + ', Rendering - ' + this.props.rendering : ''}</span>
      	</div>
    )
  }
}

export default Stats;
