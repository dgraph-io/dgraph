import React from 'react';

import loader from '../assets/images/loader.svg';


const FrameLoading = ({ message }) => {
  return (
    <div className="loading-container">
      <div className="loading-content">
        <img src={loader} alt="loading-indicator" className="loader" />
        <div className="text">Fetching result...</div>
      </div>
    </div>
  );
};

export default FrameLoading;
