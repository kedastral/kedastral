"""Prophet BYOM service implementing the Kedastral BYOM HTTP contract."""

import logging
import sys
from datetime import datetime

import pandas as pd
from flask import Flask, jsonify, request
from prophet import Prophet

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stdout
)
logger = logging.getLogger(__name__)

app = Flask(__name__)


class ProphetForecaster:
    def __init__(self):
        self.model = None
        self.last_trained = None

    def train_and_predict(self, features, horizon_seconds, step_seconds):
        if not features:
            raise ValueError("features cannot be empty")

        df = pd.DataFrame([
            {
                'ds': pd.to_datetime(f['ts']),
                'y': float(f.get('value', 0))
            }
            for f in features
        ])

        df = df.sort_values('ds').reset_index(drop=True)

        logger.info(f"Training Prophet on {len(df)} historical points")

        model = Prophet(
            interval_width=0.95,
            daily_seasonality=True,
            weekly_seasonality=True,
            yearly_seasonality=False,
            seasonality_mode='multiplicative'
        )

        model.fit(df)
        self.model = model
        self.last_trained = datetime.now()

        num_periods = horizon_seconds // step_seconds
        future_df = model.make_future_dataframe(
            periods=num_periods,
            freq=f'{step_seconds}s',
            include_history=False
        )

        forecast = model.predict(future_df)
        predictions = forecast['yhat'].apply(lambda x: max(0, x)).tolist()

        logger.info(f"Generated {len(predictions)} predictions")

        return predictions[:num_periods]


forecaster = ProphetForecaster()


@app.route('/healthz', methods=['GET'])
def healthz():
    return jsonify({
        'status': 'healthy',
        'model': 'prophet',
        'last_trained': forecaster.last_trained.isoformat() if forecaster.last_trained else None
    }), 200


@app.route('/predict', methods=['POST'])
def predict():
    """
    BYOM prediction endpoint.

    Request:
    {
        "now": "<RFC3339>",
        "horizonSeconds": 1800,
        "stepSeconds": 60,
        "features": [{"ts": "...", "value": 100.0}, ...]
    }

    Response:
    {
        "metric": "prophet_forecast",
        "values": [420.0, 415.2, ...]
    }
    """
    try:
        data = request.get_json()

        if not data:
            return jsonify({'error': 'request body is required'}), 400

        required_fields = ['horizonSeconds', 'stepSeconds', 'features']
        for field in required_fields:
            if field not in data:
                return jsonify({'error': f'missing required field: {field}'}), 400

        horizon_seconds = int(data['horizonSeconds'])
        step_seconds = int(data['stepSeconds'])
        features = data['features']

        if horizon_seconds <= 0:
            return jsonify({'error': 'horizonSeconds must be > 0'}), 400

        if step_seconds <= 0:
            return jsonify({'error': 'stepSeconds must be > 0'}), 400

        if step_seconds > horizon_seconds:
            return jsonify({'error': 'stepSeconds cannot exceed horizonSeconds'}), 400

        if not features or len(features) < 2:
            return jsonify({'error': 'features must contain at least 2 points for Prophet'}), 400

        for i, f in enumerate(features):
            if 'ts' not in f:
                return jsonify({'error': f'feature[{i}] missing required field: ts'}), 400
            if 'value' not in f:
                return jsonify({'error': f'feature[{i}] missing required field: value'}), 400

        logger.info(f"Prediction request: horizon={horizon_seconds}s, step={step_seconds}s, features={len(features)}")

        predictions = forecaster.train_and_predict(features, horizon_seconds, step_seconds)

        response = {
            'metric': 'prophet_forecast',
            'values': predictions
        }

        logger.info(f"Returning {len(predictions)} predictions")

        return jsonify(response), 200

    except ValueError as e:
        logger.error(f"Validation error: {e}")
        return jsonify({'error': str(e)}), 400
    except Exception as e:
        logger.error(f"Prediction error: {e}", exc_info=True)
        return jsonify({'error': 'internal server error'}), 500


if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8082, debug=False)
