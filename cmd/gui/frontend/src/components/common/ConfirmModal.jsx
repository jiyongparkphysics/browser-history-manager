import React from 'react'
import { motion, AnimatePresence } from 'framer-motion'

export default function ConfirmModal({ isOpen, title, message, onConfirm, onCancel, danger = false }) {
  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          className="modal-overlay"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onCancel}
        >
          <motion.div
            className="modal-card"
            initial={{ opacity: 0, scale: 0.95, y: 20 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.95, y: 20 }}
            transition={{ type: 'spring', damping: 25, stiffness: 300 }}
            onClick={e => e.stopPropagation()}
          >
            <h3 className="modal-title">{title}</h3>
            <p className="modal-message">{message}</p>
            <div className="modal-actions">
              <button className="modal-btn modal-btn-cancel" onClick={onCancel}>Cancel</button>
              <button className={`modal-btn ${danger ? 'modal-btn-danger' : 'modal-btn-confirm'}`} onClick={onConfirm}>
                {danger ? 'Delete' : 'Confirm'}
              </button>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
